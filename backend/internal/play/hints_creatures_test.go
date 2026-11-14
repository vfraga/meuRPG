package play

import (
	"context"
	"slices"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/platform/sessiontell"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
)

// The writes of the characters and campaigns modules that change what a live session shows run under play's funnel
// (Service.InSession, #735 part 2, 4c): the session's number moves once with the write, the hints go out after the commit with it, a
// refused call tells nothing and moves nothing, and with no open session nothing moves.

const kCreaturesChanged = "*playv1.WatchGameSessionResponse_CreaturesChanged_"

func (a *armed) summonersHints(t *testing.T, quiet ...string) *hintRig {
	t.Helper()
	return newHintRig(t, a.h, a.campaignID, a.master, map[string]*user{"Caio": a.caio, "Ana": a.ana, "Bia": a.bia}, quiet...)
}

func (a *armed) renameCall(t *testing.T, u *user, id, name string) func() error {
	t.Helper()
	return func() error {
		_, err := u.characters.RenameCreature(t.Context(), connect.NewRequest(&charactersv1.RenameCreatureRequest{CampaignId: a.campaignID, CreatureId: id, Name: name}))
		return err
	}
}

func (a *armed) dismissCall(t *testing.T, u *user, id string) func() error {
	t.Helper()
	return func() error {
		_, err := u.characters.DismissCreature(t.Context(), connect.NewRequest(&charactersv1.DismissCreatureRequest{CampaignId: a.campaignID, CreatureId: id}))
		return err
	}
}

// heardOnly fails unless the master and the listed players heard exactly the kind, and the others nothing.
func heardOnly(t *testing.T, heard map[string][]string, kind string, who ...string) {
	t.Helper()
	want := map[string]bool{"master": true}
	for _, w := range who {
		want[w] = true
	}
	for name, kinds := range heard {
		if want[name] {
			if len(kinds) != 1 || kinds[0] != kind {
				t.Errorf("%s heard %v, want [%s]", name, kinds, kind)
			}
		} else if len(kinds) != 0 {
			t.Errorf("%s heard %v, want nothing", name, kinds)
		}
	}
}

func (a *armed) giveCall(t *testing.T, key, monster string) func() error {
	t.Helper()
	return func() error {
		_, err := a.master.characters.GiveCreature(t.Context(), connect.NewRequest(&charactersv1.GiveCreatureRequest{
			CampaignId: a.campaignID, CharacterId: a.toren.GetId(), MonsterKey: monster, Name: "Presa", IdempotencyKey: key,
		}))
		return err
	}
}

func TestGiveCreatureHints(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	g := a.summonersHints(t)
	key := newKey()
	g.keyed(a.giveCall(t, key, "monster:wolf"), a.giveCall(t, key, "monster:wolf"), a.giveCall(t, key, "monster:bear"))
}

func TestGiveCreatureHeardByTheMasterAndTheOwner(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	g := a.summonersHints(t)
	heard := g.first(a.giveCall(t, "", "monster:wolf"), true)
	heardOnly(t, heard, kCreaturesChanged, "Caio")
}

func TestGiveCreatureRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	g := a.summonersHints(t)
	g.refused(a.giveCall(t, "", "monster:dragon-of-the-moon"))
}

func TestGiveCreatureWithNoOpenSessionMovesNothing(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	g := a.summonersHints(t)
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	before := g.seq()
	g.drain()
	if err := a.giveCall(t, "", "monster:wolf")(); err != nil {
		t.Fatalf("GiveCreature(no open session) error = %v", err)
	}
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d, want %d", got, before)
	}
}

func TestRenameCreatureHints(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	wolf := a.give(t, a.toren, "monster:wolf", "Presa")
	g := a.summonersHints(t)
	heard := g.first(a.renameCall(t, a.caio, wolf.GetId(), "Sombra"), true)
	heardOnly(t, heard, kCreaturesChanged, "Caio") // the master and the owner; nobody else
}

func TestRenameCreatureInCombatHints(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.toSalvia(t, a.summonersFight(t))
	if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	wolf := a.mustCreatures(t, a.bia, a.bri)[0]
	g := a.summonersHints(t)
	heard := g.first(a.renameCall(t, a.bia, wolf.GetId(), "Sombra"), true)
	if !slices.Contains(heard["master"], kCreaturesChanged) || !slices.Contains(heard["Ana"], "*playv1.WatchGameSessionResponse_EncounterChanged_") {
		t.Errorf("heard %v, want the master told of the creatures and the table of the combat", heard)
	}
}

func TestRenameCreatureRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	wolf := a.give(t, a.toren, "monster:wolf", "Presa")
	g := a.summonersHints(t)
	g.refused(a.renameCall(t, a.ana, wolf.GetId(), "Sombra")) // not Ana's creature
}

func TestRenameCreatureWithNoOpenSessionMovesNothing(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	wolf := a.give(t, a.toren, "monster:wolf", "Presa")
	g := a.summonersHints(t)
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	before := g.seq()
	g.drain()
	if err := a.renameCall(t, a.caio, wolf.GetId(), "Sombra")(); err != nil {
		t.Fatalf("RenameCreature(no open session) error = %v", err)
	}
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d, want %d", got, before)
	}
}

func TestDismissCreatureHints(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	wolf := a.give(t, a.toren, "monster:wolf", "Presa")
	g := a.summonersHints(t)
	heard := g.first(a.dismissCall(t, a.caio, wolf.GetId()), true)
	heardOnly(t, heard, kCreaturesChanged, "Caio")
	// Dismissing it again changes nothing: the number stays and nobody hears.
	before := g.seq()
	if err := a.dismissCall(t, a.caio, wolf.GetId())(); err != nil {
		t.Fatalf("DismissCreature(again) error = %v", err)
	}
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d after a dismissal that changed nothing, want %d", got, before)
	}
	for name, evs := range g.drain() {
		if len(evs) != 0 {
			t.Errorf("%s heard %v of a dismissal that changed nothing", name, eventKinds(evs))
		}
	}
}

func TestDismissCreatureInCombatHints(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.toSalvia(t, a.summonersFight(t))
	if _, err := a.summonWolves(t, a.bia, e, 11); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	wolf := a.mustCreatures(t, a.bia, a.bri)[0]
	g := a.summonersHints(t)
	heard := g.first(a.dismissCall(t, a.master, wolf.GetId()), true)
	if !slices.Contains(heard["master"], kCreaturesChanged) || !slices.Contains(heard["Ana"], "*playv1.WatchGameSessionResponse_EncounterChanged_") {
		t.Errorf("heard %v, want the master told of the creatures and the table of the combat", heard)
	}
}

func TestDismissCreatureRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	a.give(t, a.toren, "monster:wolf", "Presa")
	g := a.summonersHints(t)
	g.refused(a.dismissCall(t, a.caio, newKey())) // no such creature
}

func TestAdjustCreatureHitPointsHints(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	wolf := a.give(t, a.toren, "monster:wolf", "Presa")
	g := a.summonersHints(t)
	heard := g.first(func() error {
		_, err := a.master.characters.AdjustCreatureHitPoints(t.Context(), connect.NewRequest(&charactersv1.AdjustCreatureHitPointsRequest{
			CampaignId: a.campaignID, CreatureId: wolf.GetId(), Change: &charactersv1.AdjustCreatureHitPointsRequest_Damage{Damage: 2},
		}))
		return err
	}, true)
	heardOnly(t, heard, kCreaturesChanged, "Caio")
}

// A change of the table's rules moves the session's number once. In a combat the master hears that the log must be read again from
// the start (full); a player hears nothing that names a kind, only the number at the next beat.
func TestSetTableRulesHints(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	a.summonersFight(t)
	g := a.summonersHints(t, "Caio", "Ana", "Bia")
	before := g.seq()
	a.setRules(t, func(r *campaignsv1.TableRules) { r.ShowSaveDc = !r.ShowSaveDc })
	if got := g.seq(); got != before+1 {
		t.Errorf("hint_seq = %d after SetTableRules, want %d", got, before+1)
	}
	heard := g.drain()
	for name, evs := range heard {
		if name != "master" {
			if len(evs) != 0 {
				t.Errorf("%s heard %v, want nothing", name, eventKinds(evs))
			}
			continue
		}
		if len(evs) != 1 || evs[0].GetCombatLogChanged() == nil || !evs[0].GetCombatLogChanged().GetFull() || evs[0].GetSessionSeq() != before+1 {
			t.Errorf("master heard %v, want one combat_log_changed with full and the number %d", eventKinds(evs), before+1)
		}
	}
}

// With no combat the rules still move the number (a screen reads them), and nobody is told an event.
func TestSetTableRulesWithNoCombatMovesTheNumberOnly(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	g := a.summonersHints(t, "Caio", "Ana", "Bia")
	before := g.seq()
	a.setRules(t, func(r *campaignsv1.TableRules) { r.ShowSaveDc = !r.ShowSaveDc })
	if got := g.seq(); got != before+1 {
		t.Errorf("hint_seq = %d, want %d", got, before+1)
	}
	for name, evs := range g.drain() {
		if len(evs) != 0 {
			t.Errorf("%s heard %v, want nothing", name, eventKinds(evs))
		}
	}
}

// A refused set of rules (the dice mode of a campaign that does not exist for it, here an empty set of ability methods) tells nothing.
func TestSetTableRulesRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	g := a.summonersHints(t, "Caio", "Ana", "Bia")
	g.refused(func() error {
		_, err := a.master.campaigns.SetTableRules(t.Context(), connect.NewRequest(&campaignsv1.SetTableRulesRequest{CampaignId: a.campaignID, Rules: &campaignsv1.TableRules{}}))
		return err
	})
}

func TestSetTableRulesWithNoOpenSessionMovesNothing(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	g := a.summonersHints(t)
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	before := g.seq()
	a.setRules(t, func(r *campaignsv1.TableRules) { r.ShowSaveDc = !r.ShowSaveDc })
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d, want %d", got, before)
	}
}

// A write that tells only that a combat changed still moves the number: the hint is sent after the commit, so the seam touches.
func TestEncounterChangedAloneMovesTheNumber(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.summonersFight(t)
	g := a.summonersHints(t)
	g.first(func() error {
		return a.h.svc.InSession(t.Context(), a.campaignID, func(_ pgx.Tx, tell sessiontell.Tell) error {
			tell.EncounterChanged(e.GetId())
			return nil
		})
	}, true)
}

// With no open session InSession runs the write in a plain transaction and moves no number, and what the write collected for after the
// commit still goes out, with no number: the steps of After and the hints that are steps (a combat that changed). A hint that is an event
// (CreaturesChanged) has no stream to reach and goes nowhere.
func TestInSessionWithNoOpenSessionSendsWhatItCollectedWithNoNumber(t *testing.T) {
	t.Parallel()
	a := newSummoners(t)
	e := a.summonersFight(t)
	g := a.summonersHints(t)
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	g.drain()
	before := g.seq()
	err := a.h.svc.InSession(t.Context(), a.campaignID, func(_ pgx.Tx, tell sessiontell.Tell) error {
		tell.Touch()
		tell.EncounterChanged(e.GetId())
		tell.After(func(ctx context.Context, out sessiontell.Out) {
			out.Send(ctx, sessiontell.Audience{Master: true}, creaturesChangedEvent("").Message)
		})
		return nil
	})
	if err != nil {
		t.Fatalf("InSession(no open session) error = %v", err)
	}
	if got := g.seq(); got != before {
		t.Errorf("hint_seq = %d, want %d", got, before)
	}
	heard := g.drain()["master"]
	if !slices.Contains(eventKinds(heard), kCreaturesChanged) {
		t.Errorf("the master heard %v: the step of After was lost", eventKinds(heard))
	}
	if !slices.Contains(eventKinds(heard), "*playv1.WatchGameSessionResponse_EncounterChanged_") && !slices.Contains(eventKinds(heard), "*playv1.WatchGameSessionResponse_CombatLogChanged_") {
		t.Errorf("the master heard %v: the combat's change was lost", eventKinds(heard))
	}
	for _, ev := range heard {
		if ev.GetSessionSeq() != 0 {
			t.Errorf("%T carries number %d with no open session", ev.GetEvent(), ev.GetSessionSeq())
		}
	}
}
