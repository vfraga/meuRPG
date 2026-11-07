package leaktest

import (
	"bytes"
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The name of a table feature that raised a d20 goes with the roll (treated_as_source_name, #757): it reaches the roller's
// player and the master, never another player. Here the roll is the save a creature's rider makes Toren roll (CombatLogHitRider):
// the master and Caio (Toren's player) read the number, the feature's key and its name on the line of the log; Ana, the other
// player, reads the line without any of them, in the combat, in the log and in the live stream.
//
// Turn Undead (ChannelDivinityTurn) has no case of its own: the d20 of an undead's save counts by the floor of the undead's
// sheet (combat_channel.go, creatureSave), and a creature's or an NPC's sheet carries no table feature (a table's roll_minimum
// comes only from the feats of a full sheet, which has no creature type), so no name can reach that line.
const (
	riderFeatName = "Piso Zorvak4321"
	riderFloor    = 15
)

func TestARiderSaveNamesTheTableFeatureOnlyToTheMasterAndTheTargetsPlayer(t *testing.T) {
	w := newWorld(t)
	ctx := t.Context()
	// Toren holds a table feat that makes a d20 of a saving throw count as 15 at least.
	entry := must(w.master.table.CreateTableEntry(ctx, rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableFeat{TableFeat: &rulesv1.TableFeat{
			NamePt: riderFeatName, DescPt: []string{"Um talento da mesa."},
			Effects: []*rulesv1.TableEffect{{Type: "roll_minimum", Rolls: []string{"save"}, Value: strconv.Itoa(riderFloor)}},
		}},
	}))).GetEntry()
	sheet := w.toren.GetSheet()
	sheet.GetFull().FeatKeys = append(sheet.GetFull().GetFeatKeys(), entry.GetKey())
	w.toren = must(w.master.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: w.toren.GetId(), Revision: w.toren.GetRevision(), Name: w.toren.GetName(), Sheet: sheet,
	}))).GetCharacter()
	nameCanary := w.secrets.add(&canary{needle: riderFeatName, kind: "rider-minimum-name", readers: names([]*person{w.caio})})
	w.secrets.add(&canary{needle: entry.GetKey(), kind: "rider-minimum-key", readers: names([]*person{w.caio})})
	w.secrets.add(&canary{needle: strconv.Itoa(riderFloor), kind: "rider-minimum-counted", numeric: true, value: riderFloor, fields: []string{"treated_as"}, readers: names([]*person{w.caio})})
	watchers := map[*person]*streamWatcher{w.master: w.watchStream(w.master), w.ana: w.watchStream(w.ana), w.caio: w.watchStream(w.caio)}

	// The ghoul's claws hit Toren; the d20 of his Constitution save shows a 2, which counts as 15.
	w.statBlockNeedles("monster:ghoul", "claws")
	ghoul := w.monsterOnTurn("monster:ghoul", "Carniçal", false)
	key := ""
	for _, a := range must(w.master.combat.GetTurnOptions(ctx, rq(&playv1.GetTurnOptionsRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: ghoul.GetId()}))).GetOptions().GetAttacks() {
		if key == "" || a.GetAttack().GetName() == "Garras" {
			key = a.GetAttack().GetKey()
		}
	}
	rider := w.hitToren(ghoul, key)
	w.seenByAna()
	w.scriptD20(2)
	ans := w.applyRider(rider)

	riderLine := func(p *person) *playv1.CombatLogHitRider {
		var line *playv1.CombatLogHitRider
		for _, e := range w.logOfPerson(p) {
			if r := e.GetHitRider(); r != nil {
				line = r
			}
		}
		return line
	}
	// The positive controls: the master and Toren's player read the number, the key and the name of the feature.
	for who, p := range map[string]*person{"the master": w.master, "Caio": w.caio} {
		line := riderLine(p)
		if line == nil || line.GetTreatedAs() != riderFloor || line.GetTreatedAsSource() != entry.GetKey() || line.GetTreatedAsSourceName() != riderFeatName {
			t.Fatalf("%s reads the rider's line %v, want the save counted as %d by the feat %q", who, line, riderFloor, riderFeatName)
		}
		found := false
		for _, r := range w.reads(p) {
			found = found || bytes.Contains(r.body, []byte(riderFeatName))
		}
		if !found {
			t.Errorf("%s: no read of the combat holds the feature's name", who)
		}
	}
	// Ana reads the line, if any, with nothing of the roll, and no read of hers holds the name.
	if line := riderLine(w.ana); line != nil && (line.TreatedAs != nil || line.GetTreatedAsSource() != "" || line.GetTreatedAsSourceName() != "" || line.GetNumbers()) {
		t.Errorf("Ana reads the rider's line %v, want nothing of another player's save", line)
	}
	for _, r := range w.reads(w.ana) {
		if bytes.Contains(r.body, []byte(riderFeatName)) || bytes.Contains(r.body, []byte(entry.GetKey())) {
			t.Errorf("Ana's read holds the feature of Toren's save: %s", shorten(r.body))
		}
	}
	w.checkNoStatBlock(t, []reply{ans}, nil, nameCanary)

	// The stream: the master's and Caio's carry the roll, Ana's none of it.
	must(w.master.combat.EndEncounter(ctx, rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey()})))
	must(w.master.play.EndGameSession(ctx, rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
	for p, sw := range watchers {
		select {
		case <-sw.done:
		case <-time.After(30 * time.Second):
			t.Fatalf("%s's stream did not end with the session", p.name)
		}
		for _, ev := range sw.events {
			body, err := protojson.Marshal(ev)
			if err != nil {
				t.Fatalf("marshal an event: %v", err)
			}
			if p == w.ana && (bytes.Contains(body, []byte(riderFeatName)) || bytes.Contains(body, []byte(entry.GetKey()))) {
				t.Errorf("Ana: event %s holds the feature of Toren's save: %s", eventCase(ev), shorten(body))
			}
			if p == w.master {
				continue
			}
			for _, f := range w.inspect(p, reply{status: 200, body: body, msg: ev}, nil) {
				t.Errorf("%s: event %s: %s\n\tevent: %s", p.name, eventCase(ev), f, shorten(body))
			}
		}
	}
}
