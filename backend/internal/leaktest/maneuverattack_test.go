package leaktest

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A table die on a weapon attack roll (RollAttackRequest.maneuver_key): Ana's barbarian adds her subclass's d8 to a shot. The die
// is hers: the roll she gets, the master's log and her own log line list it; Caio, a third player, reads the combat
// and its log without the die, whose key is the needle. The log lines of the master and of Ana are the positive controls.

const attackDiePool = "reserva_de_golpes"

func TestAManeuverDieOnAnAttackIsTheAttackersAndTheMastersAlone(t *testing.T) {
	var dieKey string
	rt := newRollsTableWith(t, func(rt *rollsTable) {
		w, ctx := rt.world, rt.t.Context()
		entry := must(w.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
			CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableSubclass{TableSubclass: &rulesv1.TableSubclass{
				NamePt: "Caminho do Golpe", ClassKey: "class:barbarian", DescPt: []string{"Dados."},
				Levels: []*rulesv1.TableSubclassLevel{{Level: 3, Features: []*rulesv1.TableFeature{
					{NamePt: "Reserva de Golpes", DescPt: []string{"Dados."}, Effects: []*rulesv1.TableEffect{{Type: "resource", Resource: attackDiePool, Max: "2", Recharge: "short_rest"}}},
					{NamePt: "Golpe Certeiro", DescPt: []string{"Um dado."}, Effects: []*rulesv1.TableEffect{{Type: "superiority_die", Applies: "attack", Resource: attackDiePool, Value: "8"}}},
				}}},
			}},
		}))).GetEntry()
		pens := w.pens
		sheet := pens.GetSheet().GetFull()
		sheet.Classes = []*charactersv1.ClassLevel{{ClassKey: "class:barbarian", Level: 3, Subclass: &charactersv1.ClassLevel_SubclassKey{SubclassKey: entry.GetKey()}}}
		updated := must(w.ana.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
			CampaignId: w.campaign, CharacterId: pens.GetId(), Revision: pens.GetRevision(), Name: pens.GetName(), Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: sheet}},
		}))).GetCharacter()
		for _, d := range updated.GetDerived().GetRollDice() {
			dieKey = d.GetKey()
		}
		if dieKey == "" {
			panic("the barbarian has no attack die")
		}
	})
	w, m, caio, ana := rt.world, rt.master, rt.caio, rt.ana
	rt.buildRows()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the table: %v", r)
		}
	}()
	w.secrets.add(&canary{needle: dieKey, kind: "attack-die-key", readers: names([]*person{ana})})

	rt.toTurn(labelPens)
	rt.ask(t, "Ana's turn")
	// Disadvantage (the hidden goblin at her side) takes the two d20 faces; the die's 5 is the last of the extra faces.
	shot := rt.mustAttack(ana, labelPens, shortbow, "Esqueleto", []int32{14, 3}, func(r *playv1.RollAttackRequest) {
		r.ManeuverKey, r.ExtraDieFaces = dieKey, []int32{5}
	})
	if dice := shot.GetRoll().GetD20().GetExtraDice(); len(dice) != 1 || dice[0].GetFace() != 5 || dice[0].GetSourceKey() != dieKey {
		t.Fatalf("Ana's roll lists the dice %v, want her d8 of 5", dice)
	}
	rt.ask(t, "the die was added")

	// The log: the die is on the line of the master and of Ana, and on nobody else's.
	dice := func(p *person) int {
		n := 0
		for _, r := range rt.logOf(p).GetRounds() {
			for _, en := range r.GetEntries() {
				n += len(en.GetAttackRoll().GetExtraDice())
			}
		}
		return n
	}
	for _, p := range []*person{m, ana} {
		if got := dice(p); got != 1 {
			t.Errorf("%s reads %d dice on the attack line, want 1 (the positive control)", p.name, got)
		}
	}
	if got := dice(caio); got != 0 {
		t.Errorf("Caio reads %d dice on Ana's attack line, want none", got)
	}
	// The turn options list the die (maneuver_dice) with its uses: Ana and the master read it, Caio is refused.
	for _, p := range []*person{m, ana} {
		dice := rt.optionsOf(p, labelPens).GetManeuverDice()
		if len(dice) != 1 || dice[0].GetDie().GetKey() != dieKey || dice[0].GetUsesLeft() != 1 {
			t.Errorf("%s reads the maneuver dice %v, want the die with 1 use left (the positive control)", p.name, dice)
		}
	}
	res, err := caio.combat.GetTurnOptions(t.Context(), rq(&playv1.GetTurnOptionsRequest{CampaignId: w.campaign, EncounterId: rt.encounter.GetId(), CombatantId: rt.ids[labelPens]}))
	if connect.CodeOf(err) != connect.CodePermissionDenied || (res != nil && len(res.Msg.GetManeuverDice()) != 0) {
		t.Errorf("Caio reads Ana's turn options = %v, %v; want permission_denied and no die", res, err)
	}
	t.Run("canaries are reachable", func(t *testing.T) { checkCanariesAreReachable(t, w, rt.got) })
}
