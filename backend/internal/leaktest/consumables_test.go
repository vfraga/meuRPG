package leaktest

import (
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
)

// The use of a consumable outside a combat is the master's and the owner's: what a potion was, that it was used and the
// resistance it left are in the answers of nobody else. (Its line in a combat's log is the user's and the receiver's owner's
// and the master's, as the charges' lines are.)
var consumableWords = []string{"item:potion-of-resistance-fire", "Resistência a fogo", "consumable_used", "COMBAT_LOG_KIND_CONSUMABLE_USED", `"consumableUse"`}

// anaHolds puts the potion on Pensantus's sheet as the master and returns the entry's id.
func (w *world) anaHolds(key string) string { return w.holds(w.pens, key) }

// holds puts the potion on the character's sheet as the master and returns the entry's id.
func (w *world) holds(c *charactersv1.Character, key string) string {
	w.t.Helper()
	ctx := w.t.Context()
	cur := must(w.master.characters.GetCharacter(ctx, rq(&charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: c.GetId()}))).GetCharacter()
	sheet := proto.CloneOf(cur.GetSheet())
	sheet.GetFull().MagicItems = append(sheet.GetFull().MagicItems, &charactersv1.SheetMagicItem{Key: key})
	saved := must(w.master.characters.UpdateCharacter(ctx, rq(&charactersv1.UpdateCharacterRequest{
		CampaignId: w.campaign, CharacterId: c.GetId(), Revision: cur.GetRevision(), Name: cur.GetName(), Sheet: sheet,
	}))).GetCharacter()
	switch c.GetId() { // the fixture keeps the character at the revision the save left
	case w.toren.GetId():
		w.toren = saved
	case w.pens.GetId():
		w.pens = saved
	}
	for _, it := range saved.GetSheet().GetFull().GetMagicItems() {
		if it.GetKey() == key {
			return it.GetId()
		}
	}
	w.t.Fatalf("%s is not on the sheet", key)
	return ""
}

// withPotions gives Toren and Pensantus a potion of healing each, and has the NPC the master hid stand next to Toren in the
// combat, once: the rows of ListConsumableTargets ask for the list of who Toren may give the potion to with a creature nobody sees
// at his side. (A player's character does not move: its sight is the fog's, which the matrix assumes. The sheets are left alone by
// the worlds that do not ask the rows: the tests of the magic items count on the entries they make.)
func (w *world) withPotions() *world {
	if w.torenPotion != "" {
		return w
	}
	w.torenPotion, w.pensPotion = w.holds(w.toren, "item:potion-of-healing-common"), w.holds(w.pens, "item:potion-of-healing-common")
	w.torenScroll = w.holds(w.toren, "item:spell-scroll-1st")
	torenAt := w.combatant(w.toren)
	w.placeForced(w.master, w.encounter.GetId(), placement{w.combatant(w.hiddenNPC).GetId(), torenAt.GetCol() + 1, torenAt.GetRow()})
	return w
}

// withThreeFailedSaves gives Pensantus three failed death saves that the master has not confirmed, once: the players read her as
// down ("Caído") like any down ally, and the lists of receivers must tell them no more than that word.
func (w *world) withThreeFailedSaves() *world {
	if w.threeFailures {
		return w
	}
	w.threeFailures = true
	if _, err := w.pool.Exec(w.t.Context(), `UPDATE combatants SET death_failures = 3 WHERE id = $1`, w.combatant(w.pens).GetId()); err != nil {
		w.t.Fatalf("three failed death saves for Pensantus: %v", err)
	}
	return w
}

// consumableReads are the answers that could hold the use: the live session, Pensantus's sheet and the effects on characters.
// sessionReads are the reads of the session that could hold the use: the live session, the messages and the free rolls.
func (w *world) sessionReads(p *person) []reply {
	return []reply{
		p.call(playv1connect.PlayServiceGetLiveSessionProcedure, &playv1.GetLiveSessionRequest{CampaignId: w.campaign}),
		p.call(playv1connect.PlayServiceListSessionMessagesProcedure, &playv1.ListSessionMessagesRequest{CampaignId: w.campaign}),
		p.call(playv1connect.PlayServiceListFreeRollsProcedure, &playv1.ListFreeRollsRequest{CampaignId: w.campaign}),
	}
}

func (w *world) consumableReads(p *person) []reply {
	w.t.Helper()
	return append(w.sessionReads(p),
		p.call(charactersv1connect.CharacterServiceGetCharacterProcedure, &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}),
		p.call(playv1connect.LastingEffectServiceListCharacterEffectsProcedure, &playv1.ListCharacterEffectsRequest{CampaignId: w.campaign}),
	)
}

func TestAConsumableUsedOutsideACombatIsTheOwnersAndTheMastersAlone(t *testing.T) {
	w := newWorld(t)
	must(w.master.combat.EndEncounter(t.Context(), rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey()})))
	entry := w.anaHolds("item:potion-of-resistance-fire")
	// Pensantus fell in the world's combat: an unconscious creature does not act, so she is up again to drink.
	must(w.master.play.AdjustCharacterVitals(t.Context(), rq(&playv1.AdjustCharacterVitalsRequest{
		CampaignId: w.campaign, CharacterId: w.pens.GetId(), IdempotencyKey: newKey(), ExpectedRevision: w.master.revisionOf(w, w.pens.GetId()), HitPointsCurrent: proto.Int32(3),
	})))
	for _, n := range consumableWords {
		w.secrets.add(&canary{needle: n, kind: "consumable", readers: map[string]bool{w.ana.name: true}})
	}
	if other := w.caio.call(playv1connect.ChargesServiceUseConsumableProcedure, &playv1.UseConsumableRequest{
		CampaignId: w.campaign, CharacterId: w.pens.GetId(), EntryId: entry, IdempotencyKey: newKey(), RollInApp: true,
	}); other.ok() {
		t.Error("another player used Ana's potion")
	}
	res := w.ana.call(playv1connect.ChargesServiceUseConsumableProcedure, &playv1.UseConsumableRequest{
		CampaignId: w.campaign, CharacterId: w.pens.GetId(), EntryId: entry, IdempotencyKey: newKey(), RollInApp: true,
	})
	if !res.ok() {
		t.Fatalf("Ana's use: status %d %s", res.status, res.body)
	}
	if !contains(res.body, "damage-type:fire") {
		t.Errorf("the owner's answer holds no resistance: %s", res.body)
	}
	for _, p := range []*person{w.caio, w.pending, w.stranger} {
		for _, r := range w.consumableReads(p) {
			for _, f := range w.inspect(p, r, nil) {
				t.Errorf("%s: %s", p.name, f)
			}
		}
	}
	found := false
	for _, r := range w.consumableReads(w.master) {
		for _, n := range consumableWords {
			found = found || contains(r.body, n)
		}
	}
	if !found {
		t.Error("the master's reads hold no needle: the canary is unreachable")
	}
}

// In a combat the table reads that Toren gave a potion to Pensantus, with its name; the dice and the hit points it gave back are
// the master's and the two owners' (RN-10): a third player reads the line and none of its numbers.
func TestAPotionGivenInACombatShowsAThirdPlayerTheLineAndNoNumbers(t *testing.T) {
	w := &world{stack: newStack(t), secrets: newSecrets(), pts: map[string]*mapsv1.MapPoint{}, puzzles: map[string]*playv1.Puzzle{}, notes: map[string]string{}}
	ctx := t.Context()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("building the fixture: %v", r)
		}
	}()
	w.buildPeople()
	m := w.master
	w.session = must(m.play.StartGameSession(ctx, rq(&playv1.StartGameSessionRequest{CampaignId: w.campaign}))).GetGameSession().GetId()
	w.pens, w.toren = w.pc(w.ana, "Pensantus", "race:gnome"), w.pc(w.caio, "Toren", "race:human")
	dani := w.newPerson("Dani")
	w.join(false, dani)
	entry := w.holds(w.toren, "item:potion-of-healing-common")
	must(m.play.AdjustCharacterVitals(ctx, rq(&playv1.AdjustCharacterVitalsRequest{
		CampaignId: w.campaign, CharacterId: w.pens.GetId(), IdempotencyKey: newKey(), ExpectedRevision: m.revisionOf(w, w.pens.GetId()), HitPointsCurrent: proto.Int32(0),
	})))
	goblin := must(m.characters.CreateNpcFromCreature(ctx, rq(&charactersv1.CreateNpcFromCreatureRequest{
		CampaignId: w.campaign, CreatureKey: "monster:goblin", Name: "Goblin", Kind: charactersv1.CharacterKind_CHARACTER_KIND_MINION, IdempotencyKey: newKey(),
	}))).GetCharacter()
	e := must(m.combat.StartEncounter(ctx, rq(&playv1.StartEncounterRequest{
		CampaignId: w.campaign, IdempotencyKey: newKey(), Name: "Emboscada", Mode: playv1.EncounterMode_ENCOUNTER_MODE_THEATRE,
		Participants: []*playv1.Participant{{CharacterId: goblin.GetId()}},
	}))).GetEncounter()
	for _, c := range e.GetCombatants() {
		face := int32(10)
		if c.GetCharacterId() == w.toren.GetId() {
			face = 18
		}
		must(m.combat.SubmitInitiative(ctx, rq(&playv1.SubmitInitiativeRequest{
			CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: c.GetId(), IdempotencyKey: newKey(), Roll: &playv1.SubmitInitiativeRequest_D20Face{D20Face: face},
		})))
	}
	must(m.combat.BeginCombat(ctx, rq(&playv1.BeginCombatRequest{CampaignId: w.campaign, EncounterId: e.GetId(), IdempotencyKey: newKey()})))
	res := must(w.caio.charges.UseConsumable(ctx, rq(&playv1.UseConsumableRequest{
		CampaignId: w.campaign, CharacterId: w.toren.GetId(), EntryId: entry, TargetCharacterId: w.pens.GetId(), IdempotencyKey: newKey(), RollInApp: true,
	})))
	if res.GetHealed() == 0 || len(res.GetFaces()) != 2 {
		t.Fatalf("the gift = %v, want a heal rolled on two dice", res)
	}
	owners := names([]*person{w.ana, w.caio})
	for _, c := range []struct {
		kind, field string
		n           int32
	}{{"consumable-healed", "healed", res.GetHealed()}, {"consumable-roll-total", "roll_total", res.GetRollTotal()}} {
		w.secrets.add(&canary{kind: c.kind, numeric: true, value: int64(c.n), fields: []string{c.field}, readers: owners})
	}
	w.secrets.add(&canary{needle: "2d4+2", kind: "consumable-dice", readers: owners})
	logOf := func(p *person) reply {
		return p.call(playv1connect.CombatServiceListCombatLogProcedure, &playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: e.GetId()})
	}
	for _, p := range []*person{w.master, w.ana, w.caio} {
		if r := logOf(p); !contains(r.body, "2d4+2") || !contains(r.body, "consumableUse") {
			t.Errorf("%s reads no numbers in the line: %s", p.name, r.body)
		}
	}
	r := logOf(dani)
	if !contains(r.body, "consumableUse") {
		t.Fatalf("a third player reads no line: %s", r.body)
	}
	for _, f := range w.inspect(dani, r, nil) {
		t.Errorf("Dani: %s", f)
	}
	for _, p := range []*person{w.pending, w.stranger} {
		if got := logOf(p); got.ok() || contains(got.body, "consumableUse") {
			t.Errorf("%s read the log: %d %s", p.name, got.status, got.body)
		}
	}
}

// kinds are the kinds of the events a person heard, in order, from the lines of hiddenOnlyEvents.
func kinds(lines []string) []string {
	var out []string
	for _, l := range lines {
		out = append(out, strings.SplitN(l, " ", 2)[0])
	}
	return out
}

// hears fails when the person heard an event of a kind outside allowed, or none of a kind in required, or an event with a word
// of the consumable: what a third player hears of a use is a hint that names an encounter and nothing more (RN-10).
func hears(t *testing.T, who string, lines []string, allowed, required []string) {
	t.Helper()
	got := kinds(lines)
	for _, k := range got {
		if !slices.Contains(allowed, k) {
			t.Errorf("%s hears a %s, want only %v: %v", who, k, allowed, lines)
		}
	}
	for _, k := range required {
		if !slices.Contains(got, k) {
			t.Errorf("%s hears %v, want a %s", who, got, k)
		}
	}
	for _, l := range lines {
		for _, n := range append([]string{"potion", "item:", "consumable"}, consumableWords...) {
			if strings.Contains(l, n) {
				t.Errorf("%s hears %q in %s", who, n, l)
			}
		}
	}
}

// The streams after a drink in a combat: the master and the owner hear the hit points and the log, and a third player (Ana,
// whose character neither drinks nor receives) hears that the encounter and its log changed, and nothing else (RN-10).
func TestAConsumableUsedInACombatTellsAThirdPlayerOnlyThatItsLogChanged(t *testing.T) {
	w := newWorld(t)
	w.onTorenTurn()
	entry := w.holds(w.toren, "item:potion-of-healing-common")
	got := w.hiddenOnlyEvents(func() {
		must(w.caio.charges.UseConsumable(t.Context(), rq(&playv1.UseConsumableRequest{
			CampaignId: w.campaign, CharacterId: w.toren.GetId(), EntryId: entry, IdempotencyKey: newKey(), RollInApp: true,
		})))
	})
	hears(t, "Ana", got[w.ana.name], []string{"encounter_changed", "combat_log_changed"}, []string{"combat_log_changed"})
	for _, p := range []*person{w.master, w.caio} {
		hears(t, p.name, got[p.name], []string{"vitals_changed", "encounter_changed", "combat_log_changed"}, []string{"vitals_changed", "combat_log_changed"})
	}
}

// The streams after a resistance potion outside a combat: the effects of a character changed, which every player hears as the
// empty hint of any lasting effect (it carries no content); the hit points reach the master and the owner only.
func TestAResistancePotionOutsideACombatTellsAThirdPlayerNothingOfIt(t *testing.T) {
	w := newWorld(t)
	must(w.master.combat.EndEncounter(t.Context(), rq(&playv1.EndEncounterRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), IdempotencyKey: newKey()})))
	entry := w.holds(w.toren, "item:potion-of-resistance-fire")
	got := w.hiddenOnlyEvents(func() {
		must(w.caio.charges.UseConsumable(t.Context(), rq(&playv1.UseConsumableRequest{
			CampaignId: w.campaign, CharacterId: w.toren.GetId(), EntryId: entry, IdempotencyKey: newKey(), RollInApp: true,
		})))
	})
	hears(t, "Ana", got[w.ana.name], []string{"spell_casts_changed"}, []string{"spell_casts_changed"})
	for _, line := range got[w.ana.name] {
		var ev playv1.WatchGameSessionResponse
		if err := protojson.Unmarshal([]byte(strings.SplitN(line, " ", 2)[1]), &ev); err != nil || !proto.Equal(ev.GetSpellCastsChanged(), &playv1.WatchGameSessionResponse_SpellCastsChanged{}) {
			t.Errorf("Ana hears more than the empty hint: %s (%v)", line, err)
		}
	}
	for _, p := range []*person{w.master, w.caio} {
		hears(t, p.name, got[p.name], []string{"spell_casts_changed", "vitals_changed"}, []string{"spell_casts_changed", "vitals_changed"})
	}
}

// The streams after a resistance potion drunk in a combat: the effects of the character changed, so every player hears the empty
// hint of any lasting effect, besides the hints that the encounter and its log changed; none of them carries content, and the hit
// points reach the master and the owner only.
func TestAResistancePotionInACombatTellsAThirdPlayerOnlyContentlessHints(t *testing.T) {
	w := newWorld(t)
	w.onTorenTurn()
	entry := w.holds(w.toren, "item:potion-of-resistance-fire")
	got := w.hiddenOnlyEvents(func() {
		must(w.caio.charges.UseConsumable(t.Context(), rq(&playv1.UseConsumableRequest{
			CampaignId: w.campaign, CharacterId: w.toren.GetId(), EntryId: entry, IdempotencyKey: newKey(), RollInApp: true,
		})))
	})
	hears(t, "Ana", got[w.ana.name], []string{"encounter_changed", "combat_log_changed", "spell_casts_changed"}, []string{"combat_log_changed", "spell_casts_changed"})
	for _, line := range got[w.ana.name] {
		var ev playv1.WatchGameSessionResponse
		if err := protojson.Unmarshal([]byte(strings.SplitN(line, " ", 2)[1]), &ev); err != nil {
			t.Fatalf("Ana hears %q: %v", line, err)
		}
		switch strings.SplitN(line, " ", 2)[0] {
		case "spell_casts_changed":
			if !proto.Equal(ev.GetSpellCastsChanged(), &playv1.WatchGameSessionResponse_SpellCastsChanged{}) {
				t.Errorf("Ana hears more than the empty hint: %s", line)
			}
		case "encounter_changed":
			if !proto.Equal(ev.GetEncounterChanged(), &playv1.WatchGameSessionResponse_EncounterChanged{EncounterId: w.encounter.GetId(), Mode: ev.GetEncounterChanged().GetMode()}) {
				t.Errorf("Ana hears more than the encounter and its mode: %s", line)
			}
		case "combat_log_changed":
			if !proto.Equal(ev.GetCombatLogChanged(), &playv1.WatchGameSessionResponse_CombatLogChanged{EncounterId: w.encounter.GetId()}) {
				t.Errorf("Ana hears more than the encounter's id: %s", line)
			}
		}
	}
	for _, p := range []*person{w.master, w.caio} {
		hears(t, p.name, got[p.name], []string{"spell_casts_changed", "vitals_changed", "encounter_changed", "combat_log_changed"}, []string{"spell_casts_changed", "vitals_changed", "combat_log_changed"})
	}
}

// The lists of receivers of a creature with three failed death saves that the master has not confirmed: a player reads it as down
// (the word of the order of the combat) and as a character of the party outside a potion's reach, the master's list leaves it
// out, and the use is refused whatever the item before anything is spent.
func TestTheListOfReceiversTellsAPlayerOnlyTheWordOfTheOrderOfTheCombat(t *testing.T) {
	w := newWorld(t)
	w.withPotions().withThreeFailedSaves()
	// Pensantus stands next to Toren, in the reach of a potion (the matrix's world keeps the characters where the fog assumes them).
	torenAt := w.combatant(w.toren)
	w.placeForced(w.master, w.encounter.GetId(), placement{w.combatant(w.pens).GetId(), torenAt.GetCol(), torenAt.GetRow() + 1})
	list := func(u *person, entry string) []*playv1.ConsumableTarget {
		res := must(u.charges.ListConsumableTargets(t.Context(), rq(&playv1.ListConsumableTargetsRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId(), EntryId: entry})))
		return res.GetTargets()
	}
	inList := func(ts []*playv1.ConsumableTarget) *playv1.ConsumableTarget {
		for _, g := range ts {
			if g.GetCharacterId() == w.pens.GetId() {
				return g
			}
		}
		return nil
	}
	if g := inList(list(w.caio, w.torenPotion)); g == nil || g.GetState() != playv1.ConsumableTargetState_CONSUMABLE_TARGET_STATE_DOWN {
		t.Errorf("a player's list of the potion has %v for Pensantus, want down", g)
	}
	if g := inList(list(w.caio, w.torenScroll)); g == nil || g.GetState() != playv1.ConsumableTargetState_CONSUMABLE_TARGET_STATE_UNSPECIFIED {
		t.Errorf("a player's list of the scroll has %v for Pensantus, want her with no state", g)
	}
	for name, entry := range map[string]string{"potion": w.torenPotion, "scroll": w.torenScroll} {
		if g := inList(list(w.master, entry)); g != nil {
			t.Errorf("the master's list of the %s has %v, want Pensantus left out", name, g)
		}
	}
}
