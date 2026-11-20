package leaktest

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The infusions a character keeps live (the name of the infusion, the object it is on, the piece it is on, the bonus it
// gives) are the owner's and the master's, as the sheet is. A player of the table reads none of it: not in the party
// views, the combat, its log after an attack with the infused weapon (the attack's result is the only thing in it), the
// session's reads or the stream. The owner's and the master's reads are the positive controls.
//
// The table is the one of the attack rolls: Toren (Caio) is the artificer, with an infusion on his dagger and another on
// an object he names; Ana plays the barbarian.

// The fixture pack's class and what the test needs of it.
const (
	artificerPack  = "../characters/testdata/pack_engenhista.json"
	artificerClass = "class:engenhista-de-ligas@mesa"

	infusionBlade = "feature:class-engenhista-de-li-6d59b9--gume-vivo@mesa"
	infusionPlate = "feature:class-engenhista-de-li-6d59b9--couraca-de-petalas@mesa"
	infusionAsh   = "feature:class-engenhista-de-li-6d59b9--manto-de-cinza@mesa"
	infusionFalse = "feature:class-engenhista-de-li-6d59b9--fundo-falso@mesa"
	infusionSeal  = "feature:class-engenhista-de-li-6d59b9--selo-de-casa@mesa"
	// An infusion of the table's class that Toren did not pick.
	infusionFocus = "feature:class-engenhista-de-li-6d59b9--diapasao-interno@mesa"

	// The names the pack gives the blade, the plate, the seal and the focus, which the test swaps for markers.
	packBladeName, packPlateName, packSealName, packFocusName = "Gume Vivo", "Couraça de Pétalas", "Selo de Casa", "Diapasão Interno"

	// The armor Toren wears, which carries the plate, and the armor class the plate adds at his level in the copy of the pack
	// the test imports: 3, the largest bonus a piece gives, which the copy gives the plate only (the blade gives 2).
	torenArmor = "equipment:leather-armor"
	plateBonus = 3

	// The spell the casting copy of the artificer attacks with, and what its attack adds up to: the proficiency bonus (3) and
	// the Intelligence modifier (3) at level 6, and the focus (2 in the copy of the pack).
	focusCantrip                = "spell:fire-bolt"
	spellAttackBase, focusBonus = 6, 2
	kindInfusionFocus           = "infusion-focus-name"

	kindInfusionName   = "infusion-name"
	kindInfusionPlate  = "infusion-plate-name"
	kindInfusionAC     = "infusion-armor-class"
	kindInfusionSeal   = "infusion-seal-name"
	kindInfusionObject = "infusion-object-name"
	kindInfusionKey    = "infusion-key"
	kindInfusionBonus  = "infusion-bonus"

	// bladeBonus is what the blade gives Toren at his level in the copy of the pack the test imports.
	bladeBonus = 2
)

// castingPack gives the class of the pack a casting: a half caster with the Intelligence, who knows a cantrip and the spells
// of the SRD the test names, with the columns of its rows filled for the 20 levels.
func castingPack(data []byte) []byte {
	var pack map[string]any
	if err := json.Unmarshal(data, &pack); err != nil {
		panic(err)
	}
	class := pack["entries"].([]any)[0].(map[string]any)["tableClass"].(map[string]any)
	class["casting"] = map[string]any{
		"kind": "half", "startLevel": 1, "ability": "ABILITY_INTELLIGENCE", "preparation": "known", "srdSpellKeys": []string{focusCantrip, "spell:cure-wounds"},
	}
	for _, row := range class["levels"].([]any) {
		r := row.(map[string]any)
		r["cantripsKnown"], r["spellsKnown"], r["slots"] = 2, 3, []int{4, 3, 0, 0, 0, 0, 0, 0, 0}
	}
	out, err := json.Marshal(pack)
	if err != nil {
		panic(err)
	}
	return out
}

// plateBonuses gives the plate of the pack its own bonus, plateBonus at the levels of the test: the armor class it adds is a
// number no other infusion of the table gives, so it can be searched.
func plateBonuses(node any) {
	switch n := node.(type) {
	case map[string]any:
		if n["key"] == infusionPlate {
			n["infusion"].(map[string]any)["bonusSteps"] = []any{map[string]any{"classLevel": 2, "bonus": plateBonus}}
			return
		}
		for _, v := range n {
			plateBonuses(v)
		}
	case []any:
		for _, v := range n {
			plateBonuses(v)
		}
	}
}

// artificerPackWith is the fixture pack with the names of four of its infusions replaced; casts gives the class a casting.
func artificerPackWith(bladeName, plateName, sealName, focusName string, casts bool) *rulesv1.TableContentPack {
	data, err := os.ReadFile(artificerPack)
	if err != nil {
		panic(err)
	}
	data = bytes.ReplaceAll(data, []byte(packBladeName), []byte(bladeName))
	data = bytes.ReplaceAll(data, []byte(packPlateName), []byte(plateName))
	data = bytes.ReplaceAll(data, []byte(packSealName), []byte(sealName))
	data = bytes.ReplaceAll(data, []byte(packFocusName), []byte(focusName))
	// The pack's bonuses are 1 and 2: a 1 or a 2 turns up in numeric fields everywhere, so the copy gives 2 and 3 (the table's
	// largest), and the blade reads 2 at the artificer's level. The canary of that number is searched in the fields named
	// magic_bonus only (see importArtificer's caller), where nothing else of this table holds a 2.
	data = bytes.ReplaceAll(data, []byte(`"bonus": 2`), []byte(`"bonus": 3`))
	data = bytes.ReplaceAll(data, []byte(`"bonus": 1`), []byte(`"bonus": 2`))
	var tree map[string]any
	if err := json.Unmarshal(data, &tree); err != nil {
		panic(err)
	}
	plateBonuses(tree)
	data, err = json.Marshal(tree)
	if err != nil {
		panic(err)
	}
	if casts {
		data = castingPack(data)
	}
	pack := &rulesv1.TableContentPack{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, pack); err != nil {
		panic(err)
	}
	return pack
}

// importArtificer gives the table the artificer class, with the names of the blade, the plate, the seal and the focus as the
// markers given.
func (w *world) importArtificer(bladeName, plateName, sealName, focusName string, casts bool) {
	must(w.master.table.ImportTableContent(w.t.Context(), rq(&rulesv1.ImportTableContentRequest{
		CampaignId: w.campaign, Pack: artificerPackWith(bladeName, plateName, sealName, focusName, casts), Mode: rulesv1.TableImportMode_TABLE_IMPORT_MODE_APPLY, IdempotencyKey: newKey(),
	})))
}

// artificerSheet is the sheet of an artificer of level 6 who knows five infusions and carries the weapons.
func (w *world) artificerSheet(casts bool, weapons ...string) *charactersv1.CharacterSheet {
	sheet := w.fullSheet(artificerClass, "race:human", 6)
	full := sheet.GetFull()
	full.WeaponKeys = weapons
	full.ArmorKey = torenArmor
	full.SkillProficiencyKeys = []string{"skill:arcana", "skill:history"}
	full.FeatureChoiceKeys = []string{infusionBlade, infusionPlate, infusionAsh, infusionFalse, infusionSeal}
	if casts {
		full.FeatureChoiceKeys = []string{infusionBlade, infusionPlate, infusionSeal, infusionFalse, infusionFocus}
		full.CantripKeys = []string{focusCantrip}
		full.KnownSpellKeys, full.PreparedSpellKeys = []string{"spell:cure-wounds"}, []string{"spell:cure-wounds"}
	}
	return sheet
}

// infuse puts an infusion on the piece of the character's gear (or on an object, when itemKey is empty), as p.
func (w *world) infuse(p *person, characterID, infusionKey, itemKey, object string) *charactersv1.SheetInfusion {
	res := must(p.infusions.InfuseItem(w.t.Context(), rq(&charactersv1.InfuseItemRequest{
		CampaignId: w.campaign, CharacterId: characterID, InfusionKey: infusionKey, HolderCharacterId: characterID,
		ItemKey: itemKey, ObjectNamePt: object, IdempotencyKey: newKey(),
	})))
	return res.GetInfusion()
}

// infusedTable is the table of the attack rolls with Toren as the artificer.
type infusedTable struct {
	*rollsTable
	bladeName, plateName, sealName, objectName string
	blade, plate, seal                         *charactersv1.SheetInfusion
	// focusName and focus are the focus of the casting table: an object Toren casts through, attuned.
	focusName, focusObject string
	focus                  *charactersv1.SheetInfusion
}

// newInfusedTable builds the table: the class is the table's content, Toren is the artificer and his dagger and an object
// carry the blade and the seal. The names and the records are canaries the owner may read.
func newInfusedTable(t *testing.T) *infusedTable {
	t.Helper()
	return newInfusedTableWith(t, false)
}

// newInfusedTableWith is newInfusedTable; with casts, Toren's class casts and a focus he is attuned to is the third record.
func newInfusedTableWith(t *testing.T, casts bool) *infusedTable {
	t.Helper()
	it := &infusedTable{}
	it.rollsTable = newRollsTableWith(t, func(rt *rollsTable) {
		it.rollsTable = rt
		w := rt.world
		it.bladeName = w.secrets.marker(kindInfusionName, w.caio)
		it.plateName = packPlateName // the plate has a marker only on the table that wears it (the casting table keeps four records)
		if !casts {
			it.plateName = w.secrets.marker(kindInfusionPlate, w.caio)
		}
		it.sealName = w.secrets.marker(kindInfusionSeal, w.caio)
		it.objectName = w.secrets.marker(kindInfusionObject, w.caio)
		// The keys of the infusions Toren keeps, and the bonus the blade gives, are the owner's and the master's. A key is the
		// whole text of the option's key, which no other answer holds. The bonus is a small number, so it is searched only in
		// the fields named magic_bonus (the counting +N of an attack), never in the JSON text and never in a total.
		w.secrets.add(&canary{needle: infusionBlade, kind: kindInfusionKey, readers: names([]*person{w.caio})})
		w.secrets.add(&canary{needle: infusionSeal, kind: kindInfusionKey, readers: names([]*person{w.caio})})
		w.secrets.add(&canary{needle: strconv.Itoa(bladeBonus), kind: kindInfusionBonus, numeric: true, value: bladeBonus, fields: []string{"magic_bonus"}, readers: names([]*person{w.caio})})
		if !casts { // the plate is the armor class: the number is searched in the modifier lines (value), where only it gives 3
			w.secrets.add(&canary{needle: infusionPlate, kind: kindInfusionKey, readers: names([]*person{w.caio})})
			w.secrets.add(&canary{needle: strconv.Itoa(plateBonus), kind: kindInfusionAC, numeric: true, value: plateBonus, fields: []string{"value"}, readers: names([]*person{w.caio})})
		}
		it.focusName = packFocusName // the focus has a marker only on the casting table
		if casts {
			it.focusName = w.secrets.marker(kindInfusionFocus, w.caio)
		}
		w.importArtificer(it.bladeName, it.plateName, it.sealName, it.focusName, casts)
		w.toren = must(w.caio.characters.UpdateCharacter(w.t.Context(), rq(&charactersv1.UpdateCharacterRequest{
			CampaignId: w.campaign, CharacterId: w.toren.GetId(), Revision: w.toren.GetRevision(), Name: w.toren.GetName(), Sheet: w.artificerSheet(casts, dagger, shortbow),
		}))).GetCharacter()
		it.blade = w.infuse(w.caio, w.toren.GetId(), infusionBlade, dagger, "")
		it.seal = w.infuse(w.caio, w.toren.GetId(), infusionSeal, "", it.objectName)
		if !casts {
			it.plate = w.infuse(w.caio, w.toren.GetId(), infusionPlate, torenArmor, "")
			w.secrets.id("infusion-plate-record", it.plate.GetId(), w.caio)
		}
		w.secrets.id("infusion-record", it.blade.GetId(), w.caio)
		w.secrets.id("infusion-seal-record", it.seal.GetId(), w.caio)
		if casts {
			it.focusObject = w.secrets.marker(kindInfusionObject, w.caio)
			w.secrets.add(&canary{needle: infusionFocus, kind: kindInfusionKey, readers: names([]*person{w.caio})})
			res := must(w.master.infusions.InfuseItem(w.t.Context(), rq(&charactersv1.InfuseItemRequest{ // the master has no rest to wait for
				CampaignId: w.campaign, CharacterId: w.toren.GetId(), InfusionKey: infusionFocus, HolderCharacterId: w.toren.GetId(),
				ObjectNamePt: it.focusObject, AttuneNow: true, IdempotencyKey: newKey(),
			})))
			it.focus = res.GetInfusion()
			w.secrets.id("infusion-focus-record", it.focus.GetId(), w.caio)
		}
	})
	return it
}

// buildInfusionRows adds to the table's reads the ones that show the sheet and the session.
func (it *infusedTable) buildInfusionRows() {
	it.buildRows()
	sheet := func(procedure string, req func(w *world) proto.Message) read {
		return read{procedure: procedure, label: "Toren", allow: onlyCaio, why: "the sheet is the owner's and the master's", req: req}
	}
	it.rows = append(it.rows,
		sheet(charactersv1connect.CharacterServiceGetCharacterProcedure, func(w *world) proto.Message {
			return &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId()}
		}),
		sheet(charactersv1connect.InfusionServiceListCharacterInfusionsProcedure, func(w *world) proto.Message {
			return &charactersv1.ListCharacterInfusionsRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId()}
		}),
		read{
			procedure: playv1connect.CastingServiceGetCastOptionsProcedure, label: "Toren", allow: onlyCaio, why: "a player reads their own character's options: the spell attack of the artificer is the owner's and the master's",
			req: func(w *world) proto.Message {
				return &playv1.GetCastOptionsRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId()}
			},
		},
		read{
			procedure: charactersv1connect.CharacterServiceListCharactersProcedure, allow: membersAndPending, why: "the party's list: it names the characters, not their infusions",
			req: func(w *world) proto.Message { return &charactersv1.ListCharactersRequest{CampaignId: w.campaign} },
		},
		read{
			procedure: playv1connect.PlayServiceGetLiveSessionProcedure, allow: members,
			req: func(w *world) proto.Message { return &playv1.GetLiveSessionRequest{CampaignId: w.campaign} },
		},
		read{
			procedure: playv1connect.PlayServiceListGameSessionsProcedure, allow: members,
			req: func(w *world) proto.Message { return &playv1.ListGameSessionsRequest{CampaignId: w.campaign} },
		},
	)
}

// torenAttack is the line of the dagger in the options of Toren's turn, as p reads them.
func (it *infusedTable) torenAttack(p *person) *rulesv1.Attack {
	for _, a := range it.optionsOf(p, labelToren).GetOptions().GetAttacks() {
		if a.GetAttack().GetKey() == dagger {
			return a.GetAttack()
		}
	}
	panic("no dagger among the attacks of Toren as " + p.name + " reads them")
}

// sessionEvents counts the session events of the infusions the table has written.
func (it *infusedTable) sessionEvents(kind string) int {
	var n int
	if err := it.pool.QueryRow(it.t.Context(), `SELECT count(*) FROM session_events WHERE kind = $1`, kind).Scan(&n); err != nil {
		it.t.Fatalf("count the events of kind %s: %v", kind, err)
	}
	return n
}

func TestAnInfusionIsTheOwnersAndTheMastersAlone(t *testing.T) {
	it := newInfusedTable(t)
	w, m, caio, ana := it.world, it.master, it.caio, it.ana
	ctx := t.Context()
	it.buildInfusionRows()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the table: %v", r)
		}
	}()
	watchers := map[*person]*streamWatcher{m: w.watchStream(m), ana: w.watchStream(ana), caio: w.watchStream(caio)}
	it.toTurn(labelToren)
	it.ask(t, "the infusions are on")

	// The owner and the master read the infusion on the dagger and its bonus; Ana reads the dagger with neither.
	for _, p := range []*person{caio, m} {
		a := it.torenAttack(p)
		if a.GetInfusionNamePt() != it.bladeName || a.GetAttackBonus() == 0 {
			t.Fatalf("%s reads the dagger as %v, want the infusion %q (the positive control)", p.name, a, it.bladeName)
		}
	}
	infused := it.torenAttack(caio).GetAttackBonus()

	// An attack with the infused dagger: the log carries the attack's result and no word of the infusion.
	hit := it.mustAttack(caio, labelToren, dagger, "Esqueleto", []int32{17}, nil)
	if hit.GetPendingDamage() == nil {
		t.Fatalf("the attack with the dagger = %v, want a hit", hit.GetRoll())
	}
	it.damage(caio, hit.GetPendingDamage().GetId(), func(r *playv1.RollDamageRequest) {
		r.ExtrasChosen, r.Roll = true, &playv1.RollDamageRequest_TypedSum{TypedSum: 3}
	})
	it.ask(t, "the infused dagger hit")
	for _, p := range []*person{ana, caio, m} {
		if line := it.attackLine(p, labelToren, "Esqueleto"); line == nil {
			t.Errorf("%s reads no line of the infused attack in the log", p.name)
		}
	}
	it.endTurn(caio)

	// The infusion moves to the bow: the blade goes on another weapon, and the dagger is plain again.
	it.toTurn(labelToren)
	moved := must(caio.infusions.MoveInfusion(ctx, rq(&charactersv1.MoveInfusionRequest{
		CampaignId: w.campaign, CharacterId: w.toren.GetId(), InfusionId: it.blade.GetId(), ItemKey: shortbow, IdempotencyKey: newKey(),
	}))).GetInfusion()
	if moved.GetItemKey() != shortbow {
		t.Fatalf("the infusion moved to %q, want the bow", moved.GetItemKey())
	}
	it.ask(t, "the blade moved")
	for _, p := range []*person{caio, m} {
		if a := it.torenAttack(p); a.GetInfusionNamePt() != "" || a.GetAttackBonus() >= infused {
			t.Errorf("%s reads the dagger as %v after the infusion moved, want it plain", p.name, a)
		}
	}

	// The infusion ends in the middle of the combat: the next options of the turn have no name and no bonus, for anyone.
	must(caio.infusions.EndInfusion(ctx, rq(&charactersv1.EndInfusionRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId(), InfusionId: it.blade.GetId()})))
	it.ask(t, "the blade ended")
	for _, p := range []*person{caio, m} {
		for _, a := range it.optionsOf(p, labelToren).GetOptions().GetAttacks() {
			if a.GetAttack().GetInfusionNamePt() != "" {
				t.Errorf("%s reads the infusion %q after it ended", p.name, a.GetAttack().GetInfusionNamePt())
			}
		}
		if got := it.torenAttack(p).GetAttackBonus(); got >= infused {
			t.Errorf("%s reads the attack bonus %d after the infusion ended, want less than the %d it gave", p.name, got, infused)
		}
	}
	// The master ends the one on the object: the owner's list loses it.
	must(m.infusions.EndInfusion(ctx, rq(&charactersv1.EndInfusionRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId(), InfusionId: it.seal.GetId()})))
	it.ask(t, "the seal ended")

	// The session wrote its events: three infused, one moved and two ended (the plate stays on the armor). No read lists them to anyone.
	for kind, want := range map[string]int{"infusion_infused": 3, "infusion_moved": 1, "infusion_ended": 2} {
		if got := it.sessionEvents(kind); got != want {
			t.Errorf("the session holds %d events of kind %s, want %d", got, kind, want)
		}
	}

	// The end: the stream told the master and Caio of the three changes and Ana of none, and carried none of it.
	must(m.combat.EndEncounter(ctx, rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: it.encounter.GetId(), IdempotencyKey: newKey()})))
	must(m.play.EndGameSession(ctx, rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
	summary := read{
		procedure: playv1connect.PlayServiceGetSessionSummaryProcedure, allow: members,
		req: func(w *world) proto.Message {
			return &playv1.GetSessionSummaryRequest{CampaignId: w.campaign, GameSessionId: w.session}
		},
	}
	t.Run("the session summary", func(t *testing.T) { w.runRead(t, summary, it.got) })
	for p, sw := range watchers {
		select {
		case <-sw.done:
		case <-time.After(30 * time.Second):
			t.Fatalf("%s's stream did not end with the session", p.name)
		}
		hints := 0
		for _, ev := range sw.events {
			if h := ev.GetCharacterInfusionsChanged(); h != nil {
				hints++
				if h.GetCharacterId() != w.toren.GetId() {
					t.Errorf("%s's stream hears the infusions of %s, want Toren's", p.name, h.GetCharacterId())
				}
			}
			body, err := protojson.Marshal(ev)
			if err != nil {
				t.Fatalf("marshal an event: %v", err)
			}
			it.got.keep(p, reply{status: 200, body: body, msg: ev})
			if p == m {
				continue
			}
			for _, f := range w.inspect(p, reply{status: 200, body: body, msg: ev}, nil) {
				t.Errorf("%s: event %s: %s\n\tevent: %s", p.name, eventCase(ev), f, shorten(body))
			}
			if strings.Contains(string(body), "nfusion") && ev.GetCharacterInfusionsChanged() == nil {
				t.Errorf("%s: event %s speaks of an infusion: %s", p.name, eventCase(ev), shorten(body))
			}
		}
		switch want := p != ana; {
		case want && hints != 3:
			t.Errorf("%s's stream heard %d infusion hints, want 3 (moved, ended, ended)", p.name, hints)
		case !want && hints != 0:
			t.Errorf("%s's stream heard %d infusion hints, want none", p.name, hints)
		}
	}
	t.Run("canaries are reachable", func(t *testing.T) { checkCanariesAreReachable(t, w, it.got) })
}

// The targets of an infusion the character does not know: the table has the infusion, Toren did not pick it, and the answer
// is the whole answer of a key the table does not have, for the owner and for the master.
func TestInfusionTargetsOfAnInfusionTheCharacterDoesNotKnowAnswerAsAMissingKey(t *testing.T) {
	it := newInfusedTable(t)
	w := it.world
	answer := func(p *person, key string) string {
		res, err := p.infusions.ListInfusionTargets(t.Context(), rq(&charactersv1.ListInfusionTargetsRequest{
			CampaignId: w.campaign, CharacterId: w.toren.GetId(), InfusionKey: key, HolderCharacterId: w.toren.GetId(),
		}))
		if err != nil {
			return "error: " + err.Error()
		}
		body, err := protojson.Marshal(res.Msg)
		if err != nil {
			t.Fatalf("marshal the answer: %v", err)
		}
		return "ok: " + string(body)
	}
	for _, p := range []*person{w.caio, w.master} {
		if known := answer(p, infusionBlade); !strings.Contains(known, "targets") {
			t.Fatalf("%s reads the targets of a known infusion as %s, want rows (the positive control)", p.name, known)
		}
		missing := answer(p, "feature:no-such-infusion")
		if got := answer(p, infusionFocus); got != missing {
			t.Errorf("%s: the answer for an infusion the character does not know is %s, want the answer for a missing key, %s", p.name, got, missing)
		}
	}
}

// cantripAttack is the line of the cantrip in the options of Toren's turn, as p reads them; nil when p reads none.
func (it *infusedTable) cantripAttack(p *person) *rulesv1.Attack {
	res, err := p.combat.GetTurnOptions(it.t.Context(), rq(&playv1.GetTurnOptionsRequest{CampaignId: it.campaign, EncounterId: it.encounter.GetId(), CombatantId: it.ids[labelToren]}))
	if err != nil {
		return nil
	}
	for _, a := range res.Msg.GetOptions().GetAttacks() {
		if a.GetAttack().GetKey() == focusCantrip {
			return a.GetAttack()
		}
	}
	return nil
}

// The focus an artificer casts through adds to the spell attack: the owner and the master read the sum, and no other player
// reads the infusion, its object, its key or the number, in the party's reads, the combat, its log after the spell attack
// or the stream.
func TestTheSpellAttackOfAnInfusedArtificerIsTheOwnersAndTheMastersAlone(t *testing.T) {
	it := newInfusedTableWith(t, true)
	w, m, caio, ana := it.world, it.master, it.caio, it.ana
	it.buildInfusionRows()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the table: %v", r)
		}
	}()
	watchers := map[*person]*streamWatcher{m: w.watchStream(m), ana: w.watchStream(ana), caio: w.watchStream(caio)}
	it.toTurn(labelToren)
	it.ask(t, "the focus is attuned")

	// The positive control: the owner and the master read the spell attack with the focus in it.
	for _, p := range []*person{caio, m} {
		if a := it.cantripAttack(p); a == nil || a.GetAttackBonus() != spellAttackBase+focusBonus {
			t.Fatalf("%s reads the spell attack as %v, want a bonus of %d (the %d of the focus included)", p.name, a, spellAttackBase+focusBonus, focusBonus)
		}
	}
	// Ana reads nothing of Toren's turn, or at least no spell attack with the focus's number in it.
	if a := it.cantripAttack(ana); a != nil {
		t.Errorf("Ana reads the spell attack of Toren: %v", a)
	}

	hit := it.mustAttack(caio, labelToren, focusCantrip, "Esqueleto", []int32{17, 17}, nil)
	it.keepReply(t, caio, "the spell attack", hit)
	if hit.GetPendingDamage() == nil {
		t.Fatalf("the spell attack = %v, want a hit", hit.GetRoll())
	}
	it.damage(caio, hit.GetPendingDamage().GetId(), func(r *playv1.RollDamageRequest) {
		r.ExtrasChosen, r.Roll = true, &playv1.RollDamageRequest_TypedSum{TypedSum: 5}
	})
	it.ask(t, "the spell attack hit")
	if line := it.attackLine(m, labelToren, "Esqueleto"); line == nil {
		t.Fatal("the master reads no line of the spell attack in the log")
	}
	for _, needle := range []string{it.focusName, it.focusObject, infusionFocus} {
		if log := protojson.Format(it.logOf(ana)); strings.Contains(log, needle) {
			t.Errorf("Ana reads %q in the log after the spell attack", needle)
		}
	}
	it.endTurn(caio)

	// The focus ends: the number is gone for the owner and the master.
	must(m.infusions.EndInfusion(t.Context(), rq(&charactersv1.EndInfusionRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId(), InfusionId: it.focus.GetId()})))
	it.toTurn(labelToren)
	it.ask(t, "the focus ended")
	for _, p := range []*person{caio, m} {
		if a := it.cantripAttack(p); a == nil || a.GetAttackBonus() != spellAttackBase {
			t.Errorf("%s reads the spell attack as %v after the focus ended, want %d", p.name, a, spellAttackBase)
		}
	}
	must(m.combat.EndEncounter(t.Context(), rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: it.encounter.GetId(), IdempotencyKey: newKey()})))
	must(m.play.EndGameSession(t.Context(), rq(&playv1.EndGameSessionRequest{CampaignId: w.campaign, GameSessionId: w.session})))
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
			it.got.keep(p, reply{status: 200, body: body, msg: ev})
			if p == m {
				continue
			}
			for _, f := range w.inspect(p, reply{status: 200, body: body, msg: ev}, nil) {
				t.Errorf("%s: event %s: %s\n\tevent: %s", p.name, eventCase(ev), f, shorten(body))
			}
		}
	}
	t.Run("canaries are reachable", func(t *testing.T) { checkCanariesAreReachable(t, w, it.got) })
}
