package play

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The events the core handlers write, frozen on one story: a start, the initiatives, a
// reinforcement, the hiding, a surprise, the order of a tie, the begin, the conditions, the
// monsters, a removal and the end. The ids of the payloads are replaced by the combatants'
// labels (see eventTrace) and the digest of the start by a placeholder, so the golden does
// not depend on random ids.

var digestPattern = regexp.MustCompile(`"digest": "[0-9a-f]+"`)

func TestCoreStoryWritesTheseEvents(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	ctx := t.Context()
	var trace []string
	mark := a.events(t)
	var e *playv1.Encounter
	// step runs one call and adds the events it wrote to the trace, under its name.
	step := func(name string, call func() error) {
		t.Helper()
		if err := call(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		e = a.get(t, a.master)
		for _, line := range a.eventTrace(t, e, mark) {
			trace = append(trace, name+": "+digestPattern.ReplaceAllString(line, `"digest": "D"`))
			mark++
		}
	}
	// view adds what a viewer reads of the combat: who is there, in order, with the initiative
	// total, whether hidden and the conditions.
	view := func(name string, u *user) {
		t.Helper()
		var parts []string
		for _, c := range a.get(t, u).GetCombatants() {
			parts = append(parts, fmt.Sprintf("%s/%d/%v/%v", c.GetLabel(), c.GetInitiative(), c.GetHidden(), c.GetConditions()))
		}
		trace = append(trace, "view "+name+": "+strings.Join(parts, " "))
	}
	label := func(l string) string { return byLabel(t, a.get(t, a.master), l).GetId() }
	// The names of the combatants only exist once the start ran.
	a.h.roller.queue(14, 14)
	step("start", func() error {
		_, err := a.master.combat.StartEncounter(ctx, connect.NewRequest(&playv1.StartEncounterRequest{
			CampaignId: a.campaignID, IdempotencyKey: newKey(), Name: "Emboscada",
			Participants: []*playv1.Participant{{CharacterId: a.goblin.GetId(), Count: 2}},
		}))
		return err
	})
	submit := func(u *user, who string, roll func(*playv1.SubmitInitiativeRequest)) func() error {
		return func() error {
			r := &playv1.SubmitInitiativeRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: label(who), IdempotencyKey: newKey()}
			roll(r)
			_, err := u.combat.SubmitInitiative(ctx, connect.NewRequest(r))
			return err
		}
	}
	face := func(n int32) func(*playv1.SubmitInitiativeRequest) {
		return func(r *playv1.SubmitInitiativeRequest) { r.Roll = &playv1.SubmitInitiativeRequest_D20Face{D20Face: n} }
	}
	a.h.roller.queue(12)
	step("toren in the app", submit(a.caio, "Toren", func(r *playv1.SubmitInitiativeRequest) {
		r.Roll = &playv1.SubmitInitiativeRequest_RollInApp{RollInApp: true}
	}))
	step("pensantus typed", submit(a.ana, "Pensantus", face(9)))
	step("brisa by the master", submit(a.master, "Brisa", face(4)))
	a.h.roller.queue(3)
	step("add a goblin", func() error {
		_, err := a.master.combat.AddCombatants(ctx, connect.NewRequest(&playv1.AddCombatantsRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(),
			Participants: []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		}))
		return err
	})
	hide := func(who string, hidden bool) func() error {
		return func() error {
			_, err := a.master.combat.SetCombatantHidden(ctx, connect.NewRequest(&playv1.SetCombatantHiddenRequest{
				CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: label(who), IdempotencyKey: newKey(), Hidden: hidden,
			}))
			return err
		}
	}
	step("reveal goblin 1", hide("Goblin 1", false))
	step("hide goblin 1", hide("Goblin 1", true))
	step("surprise goblin 2", func() error {
		_, err := a.master.contests.SetSurprised(ctx, connect.NewRequest(&playv1.SetSurprisedRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: label("Goblin 2"), IdempotencyKey: newKey(), Surprised: true,
		}))
		return err
	})
	step("tie order", func() error {
		_, err := a.master.combat.SetInitiativeOrder(ctx, connect.NewRequest(&playv1.SetInitiativeOrderRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), CombatantIds: []string{label("Goblin 2"), label("Goblin 1")},
		}))
		return err
	})
	step("begin", func() error {
		_, err := a.master.combat.BeginCombat(ctx, connect.NewRequest(&playv1.BeginCombatRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(),
		}))
		return err
	})
	view("master after the begin", a.master)
	view("Toren's player after the begin", a.caio)
	conditions := func(u *user, who string, edit func(*playv1.SetCombatantConditionsRequest)) func() error {
		return func() error {
			r := &playv1.SetCombatantConditionsRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: label(who), IdempotencyKey: newKey()}
			edit(r)
			_, err := u.combat.SetCombatantConditions(ctx, connect.NewRequest(r))
			return err
		}
	}
	step("prone", conditions(a.master, "Goblin 1", func(r *playv1.SetCombatantConditionsRequest) {
		r.Conditions = &playv1.ConditionList{Keys: []string{condProne}}
	}))
	step("poisoned instead", conditions(a.master, "Goblin 1", func(r *playv1.SetCombatantConditionsRequest) {
		r.Conditions = &playv1.ConditionList{Keys: []string{"condition:poisoned"}}
	}))
	a.h.roller.queue(9, 6, 2, 5)
	step("two bandits rolled", func() error {
		_, err := a.addMonsters(t, e, newKey(), func(r *playv1.AddMonstersRequest) {
			r.Count, r.HitPoints, r.Name, r.Hidden = 2, playv1.MonsterHitPoints_MONSTER_HIT_POINTS_ROLLED, "Bandoleiro", proto.Bool(false)
		})
		return err
	})
	view("master with the bandits", a.master)
	view("Toren's player with the bandits", a.caio)
	step("remove a bandit", func() error {
		_, err := a.master.combat.RemoveCombatant(ctx, connect.NewRequest(&playv1.RemoveCombatantRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: label("Bandoleiro 2"), IdempotencyKey: newKey(),
		}))
		return err
	})
	step("end", func() error {
		_, err := a.master.combat.EndEncounter(ctx, connect.NewRequest(&playv1.EndEncounterRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(),
		}))
		return err
	})
	wantTrace(t, trace, coreStoryWant)
}

var coreStoryWant = []string{
	`start: encounter_started {"combatants": 5, "digest": "D", "encounter_id": "?", "map_id": "?"}`,
	`toren in the app: initiative_submitted {"combatant_id": "Toren", "face": 12, "physical": false, "total": 14}`,
	`pensantus typed: initiative_submitted {"combatant_id": "Pensantus", "face": 9, "physical": true, "total": 11}`,
	`brisa by the master: initiative_submitted {"combatant_id": "Brisa", "face": 4, "physical": true, "total": 7}`,
	`add a goblin: combatants_added {"added": 1, "hidden_added": 1}`,
	`reveal goblin 1: combatant_hidden_set {"combatant_id": "Goblin 1", "hidden": false, "round": 0}`,
	`hide goblin 1: combatant_hidden_set {"combatant_id": "Goblin 1", "hidden": true, "round": 0}`,
	`surprise goblin 2: surprise_set {"combatant_id": "Goblin 2", "contest": {"surprised": true}, "secret": true}`,
	`tie order: initiative_order_set {"combatant_ids": ["Goblin 2", "Goblin 1"]}`,
	`begin: combat_begun {"current_combatant_id": "Toren", "round": 1, "turn_group_ids": ["Toren", "Goblin 2", "Goblin 1"]}`,
	`view master after the begin: Toren/14/false/[] Goblin 2/14/true/[] Goblin 1/14/true/[] Pensantus/11/false/[] Brisa/7/false/[] Goblin 3/3/true/[]`,
	`view Toren's player after the begin: Toren/14/false/[] Pensantus/11/false/[] Brisa/7/false/[]`,
	`prone: conditions_set {"combatant_id": "Goblin 1", "cond_set": true, "conditions": ["condition:prone"], "round": 1, "secret": true}`,
	`poisoned instead: conditions_set {"combatant_id": "Goblin 1", "cond_before": ["condition:prone"], "cond_set": true, "conditions": ["condition:poisoned"], "round": 1, "secret": true}`,
	`two bandits rolled: combatants_added {"monsters": {"count": 2, "creature_key": "monster:bandit", "hidden": false, "items": [{"dice": "2d8+2", "faces": [9, 6], "hit_points": 17, "id": "Bandoleiro 1", "modifier": 2}, {"dice": "2d8+2", "faces": [2, 5], "hit_points": 9, "id": "Bandoleiro 2", "modifier": 2}], "name_hash": "18dc44a78ca24239", "rolled": true}, "round": 1}`,
	`view master with the bandits: Toren/14/false/[] Goblin 2/14/true/[] Goblin 1/14/true/[condition:poisoned] Pensantus/11/false/[] Bandoleiro 1/11/false/[] Bandoleiro 2/11/false/[] Brisa/7/false/[] Goblin 3/3/true/[]`,
	`view Toren's player with the bandits: Toren/14/false/[] Pensantus/11/false/[] Bandoleiro 1/0/false/[] Bandoleiro 2/0/false/[] Brisa/7/false/[]`,
	`remove a bandit: combatant_removed {"combatant_id": "?"}`,
	`end: encounter_ended {"round": 1}`,
}
