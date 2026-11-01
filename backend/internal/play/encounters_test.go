package play

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// The encounter builder (MR-043, RN-29). These tests need the database
// (MEURPG_TEST_DATABASE_URL). The party is Mirathel of the artboard E10-09: Toren, Pensantus and
// Brisa at level 4 and Sálvia at level 5, so the budgets are 1.250 / 1.875 / 2.600 XP.

const (
	ogre      = "monster:ogre"
	bugbear   = "monster:bugbear"
	hobgoblin = "monster:hobgoblin"
	goblin    = "monster:goblin"
	thug      = "monster:thug"
)

func groups(pairs ...any) []*playv1.MonsterGroup {
	var out []*playv1.MonsterGroup
	for len(pairs) >= 2 {
		out = append(out, &playv1.MonsterGroup{CreatureKey: pairs[0].(string), Count: n32(pairs[1].(int))})
		pairs = pairs[2:]
	}
	return out
}

// artboardOne is the encounter of the artboard's state 1: 1.550 XP, Moderada.
func artboardOne() []*playv1.MonsterGroup {
	return groups(ogre, 1, bugbear, 2, hobgoblin, 4, goblin, 6)
}

// mirathel is a campaign whose party is the artboard's: levels 4, 4, 4 and 5, with an NPC
// (Orin, the guide) and the open session. salvia is the level 5 character.
type mirathel struct {
	*armed
	salvia *charactersv1.Character
	dani   *user
	orin   *charactersv1.Character
}

func newMirathel(t *testing.T) *mirathel {
	t.Helper()
	m := &mirathel{}
	m.armed = newArmedWith(t, func(a *armed) {
		scores := &rulesv1.AbilityScores{Strength: 14, Dexterity: 12, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 4, scores, []string{battleaxe}, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 4, scores, nil, []string{fireBolt})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 4, scores, []string{rapier}, nil)
		m.dani = a.h.newUser("Dani")
		a.h.join(a.master, a.campaignID, m.dani)
		m.salvia = m.dani.hero(t, a.campaignID, "Sálvia", "class:fighter", "race:human", 5, scores, []string{battleaxe}, nil)
		m.orin = a.master.npc(t, a.campaignID, "Orin, o guia", 20, 12)
	})
	return m
}

func (m *mirathel) evaluate(t *testing.T, entries []*playv1.MonsterGroup, extra ...*playv1.PartyNpc) *playv1.EncounterEvaluation {
	t.Helper()
	res, err := m.master.encounters.EvaluateEncounter(t.Context(), connect.NewRequest(&playv1.EvaluateEncounterRequest{CampaignId: m.campaignID, Entries: entries, ExtraParty: extra}))
	if err != nil {
		t.Fatalf("EvaluateEncounter() error = %v", err)
	}
	return res.Msg.GetEvaluation()
}

// point makes a battle point on the session's map, with no map of its own to lead to: the fight is
// on the current map.
func (m *mirathel) point(t *testing.T, name string) string {
	t.Helper()
	var id string
	if err := m.h.pool.QueryRow(t.Context(),
		`INSERT INTO map_points (map_id, kind, name, x_bp, y_bp, created_at, updated_at) VALUES ($1, 'battle', $2, 100, 100, now(), now()) RETURNING id`,
		m.mapID, name).Scan(&id); err != nil {
		t.Fatalf("insert point: %v", err)
	}
	return id
}

func budgetOf(ev *playv1.EncounterEvaluation) [3]int32 {
	return [3]int32{ev.GetBudget().GetLow(), ev.GetBudget().GetModerate(), ev.GetBudget().GetHigh()}
}

// TestMR043_TheBuilderMeasuresAnEncounterAgainstTheParty: the numbers of the artboard E10-09. State 1:
// the party's budget is 1.250 / 1.875 / 2.600 and 1.550 XP is Moderada, with the cap at ND 7.
// State 2: Orin at level 3 adds 150 / 225 / 400 and lowers the cap to ND 6. State 3: 2.900 XP
// is above high, by 300, with the warning and no refusal.
func TestMR043_TheBuilderMeasuresAnEncounterAgainstTheParty(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)

	ev := m.evaluate(t, artboardOne())
	if budgetOf(ev) != [3]int32{1250, 1875, 2600} || ev.GetTotalXp() != 1550 || ev.GetCreatureCount() != 13 ||
		ev.GetBand() != playv1.EncounterBand_ENCOUNTER_BAND_MODERATE || ev.GetOverXp() != 0 || ev.GetMaxCr() != "7" || ev.GetLowestLevel() != 4 || len(ev.GetWarnings()) != 0 {
		t.Errorf("state 1: %v", ev)
	}
	if len(ev.GetParty()) != 4 || ev.GetParty()[3].GetName() != "Sálvia" || ev.GetParty()[3].GetLevel() != 5 || ev.GetParty()[0].GetNpc() {
		t.Errorf("the party = %v, want Toren, Pensantus, Brisa and Sálvia (5), all players", ev.GetParty())
	}
	if l := ev.GetLines()[0]; l.GetCreature().GetKey() != ogre || l.GetCreature().GetNamePt() != "Ogro" || l.GetCreature().GetXp() != 450 || l.GetSubtotalXp() != 450 || l.GetCreature().GetChallengeRating() != "2" {
		t.Errorf("first line = %v, want the Ogro, ND 2, 450 XP", l)
	}

	// State 2: an NPC with a level the master gives (the sheet has none).
	ev = m.evaluate(t, artboardOne(), &playv1.PartyNpc{CharacterId: m.orin.GetId(), Level: 3})
	if budgetOf(ev) != [3]int32{1400, 2100, 3000} || ev.GetMaxCr() != "6" || ev.GetLowestLevel() != 3 || ev.GetBand() != playv1.EncounterBand_ENCOUNTER_BAND_MODERATE {
		t.Errorf("with Orin: %v", ev)
	}
	if o := ev.GetParty()[4]; !o.GetNpc() || o.GetName() != "Orin, o guia" || o.GetLevel() != 3 || o.GetCharacterId() != m.orin.GetId() {
		t.Errorf("Orin in the party = %v", o)
	}
	// An NPC by name and level only.
	ev = m.evaluate(t, artboardOne(), &playv1.PartyNpc{Name: "Um mercenário", Level: 8})
	if budgetOf(ev) != [3]int32{2250, 3575, 4700} || ev.GetMaxCr() != "7" {
		t.Errorf("with a level 8 mercenary: %v", ev)
	}

	// State 3: four Ogros.
	ev = m.evaluate(t, groups(ogre, 4, bugbear, 2, hobgoblin, 4, goblin, 6))
	if ev.GetTotalXp() != 2900 || ev.GetBand() != playv1.EncounterBand_ENCOUNTER_BAND_ABOVE_HIGH || ev.GetOverXp() != 300 ||
		!slices.Contains(ev.GetWarnings(), playv1.EncounterWarning_ENCOUNTER_WARNING_ABOVE_HIGH) {
		t.Errorf("state 3: %v", ev)
	}
	// Under "Baixa" it still reads "Baixa"; nothing at all too.
	if ev = m.evaluate(t, groups(goblin, 2)); ev.GetBand() != playv1.EncounterBand_ENCOUNTER_BAND_LOW || ev.GetTotalXp() != 100 {
		t.Errorf("two goblins: %v", ev)
	}
	if ev = m.evaluate(t, nil); ev.GetBand() != playv1.EncounterBand_ENCOUNTER_BAND_LOW || ev.GetTotalXp() != 0 {
		t.Errorf("no creatures: %v", ev)
	}
	// A creature above the cap (a Troll, ND 5, for a lowest level of 1 is not this party's; an Ancient
	// Red Dragon, ND 24, is above 7): the master may keep it.
	if ev = m.evaluate(t, groups("monster:adult-red-dragon", 1)); !ev.GetLines()[0].GetAboveCap() || !slices.Contains(ev.GetWarnings(), playv1.EncounterWarning_ENCOUNTER_WARNING_ABOVE_CR_CAP) {
		t.Errorf("an adult red dragon (ND 17): %v", ev)
	}

	// A character that dies leaves the party and the budget shrinks by itself (question 86).
	if _, err := m.master.characters.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: m.campaignID, CharacterId: m.salvia.GetId()})); err != nil {
		t.Fatalf("MarkCharacterDead() error = %v", err)
	}
	ev = m.evaluate(t, artboardOne())
	if budgetOf(ev) != [3]int32{750, 1125, 1500} || len(ev.GetParty()) != 3 || ev.GetBand() != playv1.EncounterBand_ENCOUNTER_BAND_ABOVE_HIGH {
		t.Errorf("without Sálvia: %v", ev)
	}
}

// TestMR043_TheBuilderRefusals: what the master cannot ask for.
func TestMR043_TheBuilderRefusals(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	tooMany := make([]*playv1.MonsterGroup, 21)
	for i := range tooMany {
		tooMany[i] = &playv1.MonsterGroup{CreatureKey: ogre}
	}
	for name, req := range map[string]*playv1.EvaluateEncounterRequest{
		"a count of 41":                  {Entries: groups(ogre, 41)},
		"a negative count":               {Entries: []*playv1.MonsterGroup{{CreatureKey: ogre, Count: -1}}},
		"a creature twice":               {Entries: groups(ogre, 1, ogre, 2)},
		"21 entries":                     {Entries: tooMany},
		"a name of two lines":            {Entries: []*playv1.MonsterGroup{{CreatureKey: ogre, Name: "a\nb"}}},
		"an NPC at level 0":              {ExtraParty: []*playv1.PartyNpc{{Name: "x", Level: 0}}},
		"an NPC at level 21":             {ExtraParty: []*playv1.PartyNpc{{Name: "x", Level: 21}}},
		"an NPC with no name":            {ExtraParty: []*playv1.PartyNpc{{Level: 3}}},
		"a player's character as an NPC": {ExtraParty: []*playv1.PartyNpc{{CharacterId: m.toren.GetId(), Level: 3}}},
		"a character that is no one's":   {ExtraParty: []*playv1.PartyNpc{{CharacterId: newKey(), Level: 3}}},
		"an NPC twice":                   {ExtraParty: []*playv1.PartyNpc{{CharacterId: m.orin.GetId(), Level: 3}, {CharacterId: m.orin.GetId(), Level: 4}}},
		"eleven NPCs":                    {ExtraParty: slices.Repeat([]*playv1.PartyNpc{{Name: "x", Level: 1}}, 11)},
	} {
		req.CampaignId = m.campaignID
		if _, err := m.master.encounters.EvaluateEncounter(t.Context(), connect.NewRequest(req)); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("EvaluateEncounter(%s) = %v, want invalid_argument", name, err)
		}
	}
	// A key that is not an SRD creature is a typed failed_precondition, so the app can say which to take out.
	for _, key := range []string{"monster:no-such-thing", "spell:fireball"} {
		_, err := m.master.encounters.EvaluateEncounter(t.Context(), connect.NewRequest(&playv1.EvaluateEncounterRequest{CampaignId: m.campaignID, Entries: groups(key, 1)}))
		wantBuildBlocked(t, "EvaluateEncounter("+key+")", err, playv1.EncounterBuildBlockedReason_ENCOUNTER_BUILD_BLOCKED_REASON_UNKNOWN_CREATURE)
	}
	// 40 of one creature is allowed, and the party and the monsters together past 40 are a warning.
	if ev := m.evaluate(t, groups(goblin, 40)); !slices.Contains(ev.GetWarnings(), playv1.EncounterWarning_ENCOUNTER_WARNING_TOO_MANY) {
		t.Errorf("40 goblins and 4 players: %v, want TOO_MANY", ev.GetWarnings())
	}
}

func (m *mirathel) generate(t *testing.T, edit func(*playv1.GenerateEncounterRequest)) (*playv1.GenerateEncounterResponse, error) {
	t.Helper()
	req := &playv1.GenerateEncounterRequest{CampaignId: m.campaignID, Band: playv1.EncounterBand_ENCOUNTER_BAND_MODERATE, CreatureType: "humanoid", Seed: 7731}
	if edit != nil {
		edit(req)
	}
	res, err := m.master.encounters.GenerateEncounter(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func linesOf(ev *playv1.EncounterEvaluation) string {
	var parts []string
	for _, l := range ev.GetLines() {
		parts = append(parts, fmt.Sprintf("%sx%d", l.GetCreature().GetKey(), l.GetCount()))
	}
	return strings.Join(parts, " ")
}

// TestMR043_GenerateIsDeterministicAndKeepsItsPromises: "Gerar encontro", Moderada, Humanoide, for
// Mirathel (artboard 4): the same seed gives the same encounter; a leader and a group of one or two
// kinds of humanoid, within 1.875 XP and ND 7, spending nearly all of it; the NPC's level lowers the cap.
func TestMR043_GenerateIsDeterministicAndKeepsItsPromises(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	first, err := m.generate(t, nil)
	if err != nil {
		t.Fatalf("GenerateEncounter() error = %v", err)
	}
	ev := first.GetEvaluation()
	if first.GetSeed() != 7731 || ev.GetTotalXp() > 1875 || ev.GetTotalXp() < 1700 || ev.GetBand() > playv1.EncounterBand_ENCOUNTER_BAND_MODERATE ||
		len(ev.GetLines()) < 2 || len(ev.GetLines()) > 3 || ev.GetMaxCr() != "7" || budgetOf(ev) != [3]int32{1250, 1875, 2600} {
		t.Errorf("generated = seed %d, %v", first.GetSeed(), ev)
	}
	for _, l := range ev.GetLines() {
		if l.GetCreature().GetType() != "humanoid" || l.GetAboveCap() || l.GetCount() < 1 {
			t.Errorf("line %v breaks the type or the cap", l)
		}
	}
	// The leader is the creature of the highest rating, and comes first.
	for _, l := range ev.GetLines()[1:] {
		if l.GetCreature().GetXp() > ev.GetLines()[0].GetCreature().GetXp() {
			t.Errorf("%s is stronger than the leader", l.GetCreature().GetKey())
		}
	}
	for range 3 {
		again, err := m.generate(t, nil)
		if err != nil || linesOf(again.GetEvaluation()) != linesOf(ev) {
			t.Fatalf("the same seed gave %q then %q (%v)", linesOf(ev), linesOf(again.GetEvaluation()), err)
		}
	}
	// Other seeds make other encounters, never over the band's budget or above the cap, in any band.
	seen := map[string]bool{}
	for seed := uint32(1); seed <= 25; seed++ {
		for _, band := range []playv1.EncounterBand{playv1.EncounterBand_ENCOUNTER_BAND_LOW, playv1.EncounterBand_ENCOUNTER_BAND_MODERATE, playv1.EncounterBand_ENCOUNTER_BAND_HIGH} {
			res, err := m.generate(t, func(r *playv1.GenerateEncounterRequest) { r.Seed, r.Band, r.CreatureType = seed, band, "" })
			if err != nil {
				t.Fatalf("seed %d, band %v: %v", seed, band, err)
			}
			e := res.GetEvaluation()
			limit := map[playv1.EncounterBand]int32{playv1.EncounterBand_ENCOUNTER_BAND_LOW: 1250, playv1.EncounterBand_ENCOUNTER_BAND_MODERATE: 1875, playv1.EncounterBand_ENCOUNTER_BAND_HIGH: 2600}[band]
			if e.GetTotalXp() > limit || e.GetBand() > band || len(e.GetWarnings()) != 0 {
				t.Fatalf("seed %d, band %v: %d XP, band %v, warnings %v", seed, band, e.GetTotalXp(), e.GetBand(), e.GetWarnings())
			}
			seen[linesOf(e)] = true
		}
	}
	if len(seen) < 10 {
		t.Errorf("75 requests made only %d different encounters", len(seen))
	}
	// Orin at level 3 lowers the cap to ND 6, and the budget grows.
	withOrin, err := m.generate(t, func(r *playv1.GenerateEncounterRequest) {
		r.ExtraParty = []*playv1.PartyNpc{{CharacterId: m.orin.GetId(), Level: 3}}
	})
	if err != nil || withOrin.GetEvaluation().GetMaxCr() != "6" || withOrin.GetEvaluation().GetTotalXp() > 2100 || budgetOf(withOrin.GetEvaluation()) != [3]int32{1400, 2100, 3000} {
		t.Errorf("with Orin: %v, %v", withOrin, err)
	}
	// "Gerar outro": no seed asks the server to draw one (two d100: 1 to 10000), and the answer says which.
	m.h.roller.queue(37, 42)
	drawn, err := m.generate(t, func(r *playv1.GenerateEncounterRequest) { r.Seed = 0 })
	if err != nil || drawn.GetSeed() != 3642 {
		t.Errorf("a drawn seed = %v, %v; want 3642", drawn.GetSeed(), err)
	}
	replay, _ := m.generate(t, func(r *playv1.GenerateEncounterRequest) { r.Seed = drawn.GetSeed() })
	if linesOf(replay.GetEvaluation()) != linesOf(drawn.GetEvaluation()) {
		t.Errorf("the drawn seed %d did not replay the encounter", drawn.GetSeed())
	}
}

func wantBuildBlocked(t *testing.T, call string, err error, reason playv1.EncounterBuildBlockedReason) {
	t.Helper()
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("%s = %v, want failed_precondition", call, err)
	}
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok {
		t.Fatalf("%s = %v, want a Connect error", call, err)
	}
	for _, d := range ce.Details() {
		v, verr := d.Value()
		if verr != nil {
			continue
		}
		if b, ok := v.(*playv1.EncounterBuildBlocked); ok && b.GetReason() == reason {
			return
		}
	}
	t.Errorf("%s = %v, want the detail %v", call, err, reason)
}

// TestMR043_GenerateRefusals: no creature fits, an empty party, and requests that are not one.
func TestMR043_GenerateRefusals(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	for name, edit := range map[string]func(*playv1.GenerateEncounterRequest){
		"the band above high":    func(r *playv1.GenerateEncounterRequest) { r.Band = playv1.EncounterBand_ENCOUNTER_BAND_ABOVE_HIGH },
		"a band that is not one": func(r *playv1.GenerateEncounterRequest) { r.Band = playv1.EncounterBand(9) },
		"a type that is not one": func(r *playv1.GenerateEncounterRequest) { r.CreatureType = "robot" },
		"an NPC at level 0":      func(r *playv1.GenerateEncounterRequest) { r.ExtraParty = []*playv1.PartyNpc{{Name: "x"}} },
	} {
		if _, err := m.generate(t, edit); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("GenerateEncounter(%s) = %v, want invalid_argument", name, err)
		}
	}
	// A campaign with no party: nothing to build for.
	empty := m.h.newCampaign(m.master, "Vazia")
	_, err := m.master.encounters.GenerateEncounter(t.Context(), connect.NewRequest(&playv1.GenerateEncounterRequest{CampaignId: empty, Band: playv1.EncounterBand_ENCOUNTER_BAND_LOW}))
	wantBuildBlocked(t, "GenerateEncounter(no party)", err, playv1.EncounterBuildBlockedReason_ENCOUNTER_BUILD_BLOCKED_REASON_NO_PARTY)
	// ...but an NPC in the party is a party.
	res, err := m.master.encounters.GenerateEncounter(t.Context(), connect.NewRequest(&playv1.GenerateEncounterRequest{
		CampaignId: empty, Band: playv1.EncounterBand_ENCOUNTER_BAND_LOW, Seed: 5, ExtraParty: []*playv1.PartyNpc{{Name: "Orin", Level: 3}},
	}))
	if err != nil || res.Msg.GetEvaluation().GetTotalXp() > 150 || len(res.Msg.GetEvaluation().GetWarnings()) != 0 {
		t.Errorf("a party of Orin alone: %v, %v", res, err)
	}
	// A giant costs at least 450 XP: nothing fits a low encounter of a level 3 NPC alone (150 XP).
	_, err = m.master.encounters.GenerateEncounter(t.Context(), connect.NewRequest(&playv1.GenerateEncounterRequest{
		CampaignId: empty, Band: playv1.EncounterBand_ENCOUNTER_BAND_LOW, CreatureType: "giant", ExtraParty: []*playv1.PartyNpc{{Name: "Orin", Level: 3}},
	}))
	wantBuildBlocked(t, "GenerateEncounter(giants for 150 XP)", err, playv1.EncounterBuildBlockedReason_ENCOUNTER_BUILD_BLOCKED_REASON_NOTHING_FITS)
}

// TestMR043_SwapOptionsKeepTheTotal: "Trocar criatura" (artboard 5) lists the creatures with the
// same XP as the Capanga (ND 1/2, 100 XP) and, with a type, the same type: Gnoll, Hobgoblin, Orc,
// Povo-lagarto and Sahuagin are there, the Capanga itself and the Bandido (25 XP) are not.
func TestMR043_SwapOptionsKeepTheTotal(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	list := func(key, kind string) []*rulesv1.CreatureSummary {
		t.Helper()
		res, err := m.master.encounters.ListEncounterSwaps(t.Context(), connect.NewRequest(&playv1.ListEncounterSwapsRequest{CampaignId: m.campaignID, CreatureKey: key, CreatureType: kind}))
		if err != nil {
			t.Fatalf("ListEncounterSwaps(%s) error = %v", key, err)
		}
		return res.Msg.GetCreatures()
	}
	got := list(thug, "humanoid")
	var keys []string
	for _, c := range got {
		keys = append(keys, strings.TrimPrefix(c.GetKey(), "monster:"))
		if c.GetXp() != 100 || c.GetType() != "humanoid" || c.GetKey() == thug {
			t.Errorf("%s (XP %d, %s) is not a swap of the Capanga", c.GetKey(), c.GetXp(), c.GetType())
		}
	}
	for _, want := range []string{"gnoll", "hobgoblin", "orc", "lizardfolk", "sahuagin"} {
		if !slices.Contains(keys, want) {
			t.Errorf("the swaps %v lack %s", keys, want)
		}
	}
	if untyped := list(thug, ""); len(untyped) < len(got) {
		t.Errorf("without a type there are %d swaps, with one %d", len(untyped), len(got))
	}
	// The swap keeps the total: swap the 4 Capangas for 4 Orcs in an encounter.
	before := m.evaluate(t, groups(thug, 4, bugbear, 1))
	after := m.evaluate(t, groups("monster:orc", 4, bugbear, 1))
	if before.GetTotalXp() != after.GetTotalXp() {
		t.Errorf("the swap changed the total: %d, then %d", before.GetTotalXp(), after.GetTotalXp())
	}
	for name, req := range map[string]*playv1.ListEncounterSwapsRequest{
		"an unknown creature": {CreatureKey: "monster:nope"},
		"an unknown type":     {CreatureKey: thug, CreatureType: "robot"},
	} {
		req.CampaignId = m.campaignID
		if _, err := m.master.encounters.ListEncounterSwaps(t.Context(), connect.NewRequest(req)); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("ListEncounterSwaps(%s) = %v, want invalid_argument", name, err)
		}
	}
}

func (m *mirathel) save(t *testing.T, point string, be *playv1.BattleEncounter) *playv1.SaveBattleEncounterResponse {
	t.Helper()
	res, err := m.master.encounters.SaveBattleEncounter(t.Context(), connect.NewRequest(&playv1.SaveBattleEncounterRequest{CampaignId: m.campaignID, MapPointId: point, Encounter: be}))
	if err != nil {
		t.Fatalf("SaveBattleEncounter() error = %v", err)
	}
	return res.Msg
}

func (m *mirathel) read(t *testing.T, point string) *playv1.GetBattleEncounterResponse {
	t.Helper()
	res, err := m.master.encounters.GetBattleEncounter(t.Context(), connect.NewRequest(&playv1.GetBattleEncounterRequest{CampaignId: m.campaignID, MapPointId: point}))
	if err != nil {
		t.Fatalf("GetBattleEncounter() error = %v", err)
	}
	return res.Msg
}

// TestMR043_SaveReadAndClearOnABattlePoint: artboard 6. The encounter is kept with its point, read
// back measured against the party of the day, replaced by a new save, saved again with no change at
// all, and cleared (twice: not an error).
func TestMR043_SaveReadAndClearOnABattlePoint(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	bridge, ruins := m.point(t, "Emboscada na ponte"), m.point(t, "Ruínas do forte")

	if got := m.read(t, bridge); got.GetEncounter() != nil || got.GetEvaluation() != nil {
		t.Errorf("a point with no encounter = %v, want nothing", got)
	}
	be := &playv1.BattleEncounter{Monsters: artboardOne()}
	saved := m.save(t, bridge, be)
	if saved.GetEvaluation().GetTotalXp() != 1550 || saved.GetEvaluation().GetBand() != playv1.EncounterBand_ENCOUNTER_BAND_MODERATE {
		t.Errorf("saved = %v", saved.GetEvaluation())
	}
	// As kept: the modes explicit (average hit points, hidden), the counts as asked.
	if saved.GetEncounter().GetHitPoints() != playv1.MonsterHitPoints_MONSTER_HIT_POINTS_AVERAGE || !saved.GetEncounter().GetHidden() || len(saved.GetEncounter().GetMonsters()) != 4 {
		t.Errorf("kept = %v", saved.GetEncounter())
	}
	got := m.read(t, bridge)
	if got.GetEvaluation().GetTotalXp() != 1550 || got.GetEncounter().GetMonsters()[0].GetCreatureKey() != ogre || !got.GetUpdatedAt().IsValid() {
		t.Errorf("read = %v", got)
	}

	// Saving the same encounter again changes nothing, not even the time.
	again := m.save(t, bridge, &playv1.BattleEncounter{Monsters: artboardOne(), HitPoints: playv1.MonsterHitPoints_MONSTER_HIT_POINTS_AVERAGE})
	if !again.GetUpdatedAt().AsTime().Equal(saved.GetUpdatedAt().AsTime()) {
		t.Errorf("a repeated save moved the time: %v then %v", saved.GetUpdatedAt().AsTime(), again.GetUpdatedAt().AsTime())
	}
	// The list of a map's points that keep one.
	list, err := m.master.encounters.ListBattleEncounters(t.Context(), connect.NewRequest(&playv1.ListBattleEncountersRequest{CampaignId: m.campaignID, MapId: m.mapID}))
	if err != nil || len(list.Msg.GetEncounters()) != 1 || list.Msg.GetEncounters()[0].GetMapPointId() != bridge || list.Msg.GetEncounters()[0].GetCreatureCount() != 13 {
		t.Errorf("ListBattleEncounters = %v, %v; want the bridge with 13 creatures", list, err)
	}

	// A new save replaces the old one, with names, rolled hit points and not hidden.
	named := m.save(t, bridge, &playv1.BattleEncounter{
		Monsters:  []*playv1.MonsterGroup{{CreatureKey: ogre, Count: 2, Name: "Ogro da ponte"}},
		HitPoints: playv1.MonsterHitPoints_MONSTER_HIT_POINTS_ROLLED, Hidden: new(false),
	})
	if r := m.read(t, bridge); len(r.GetEncounter().GetMonsters()) != 1 || r.GetEncounter().GetMonsters()[0].GetName() != "Ogro da ponte" ||
		r.GetEncounter().GetHitPoints() != playv1.MonsterHitPoints_MONSTER_HIT_POINTS_ROLLED || r.GetEncounter().GetHidden() ||
		r.GetEvaluation().GetTotalXp() != 900 || r.GetEvaluation().GetLines()[0].GetName() != "Ogro da ponte" || !named.GetUpdatedAt().AsTime().After(saved.GetUpdatedAt().AsTime()) {
		t.Errorf("after replacing: %v", r)
	}
	// Another point has its own; reading measures against the party of the day: Sálvia dies.
	m.save(t, ruins, &playv1.BattleEncounter{Monsters: groups(goblin, 3)})
	if _, err := m.master.characters.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: m.campaignID, CharacterId: m.salvia.GetId()})); err != nil {
		t.Fatalf("MarkCharacterDead() error = %v", err)
	}
	if r := m.read(t, ruins); budgetOf(r.GetEvaluation()) != [3]int32{750, 1125, 1500} || r.GetEvaluation().GetTotalXp() != 150 {
		t.Errorf("the ruins' encounter after a death = %v", r.GetEvaluation())
	}

	// Clear: the point has none again, the other keeps its own, and clearing twice is no error.
	for range 2 {
		if _, err := m.master.encounters.ClearBattleEncounter(t.Context(), connect.NewRequest(&playv1.ClearBattleEncounterRequest{CampaignId: m.campaignID, MapPointId: bridge})); err != nil {
			t.Fatalf("ClearBattleEncounter() error = %v", err)
		}
	}
	if m.read(t, bridge).GetEncounter() != nil || m.read(t, ruins).GetEncounter() == nil {
		t.Error("clearing one point touched the other or left its own")
	}

	// A point that is not a battle point of the campaign, or a bad encounter, is refused.
	var scene string
	if err := m.h.pool.QueryRow(t.Context(), `INSERT INTO map_points (map_id, kind, name, x_bp, y_bp, created_at, updated_at) VALUES ($1, 'scene', 'Taverna', 1, 1, now(), now()) RETURNING id`, m.mapID).Scan(&scene); err != nil {
		t.Fatalf("insert point: %v", err)
	}
	for _, id := range []string{scene, newKey(), "not-a-uuid"} {
		if _, err := m.master.encounters.SaveBattleEncounter(t.Context(), connect.NewRequest(&playv1.SaveBattleEncounterRequest{CampaignId: m.campaignID, MapPointId: id, Encounter: &playv1.BattleEncounter{Monsters: groups(goblin, 1)}})); connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("SaveBattleEncounter(point %q) = %v, want not_found", id, err)
		}
	}
	other := newMirathel(t)
	foreign := other.point(t, "De outra mesa")
	if _, err := m.master.encounters.GetBattleEncounter(t.Context(), connect.NewRequest(&playv1.GetBattleEncounterRequest{CampaignId: m.campaignID, MapPointId: foreign})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("GetBattleEncounter(a point of another campaign) = %v, want not_found", err)
	}
	for name, be := range map[string]*playv1.BattleEncounter{
		"no creature":      {},
		"hit points of 9":  {Monsters: groups(goblin, 1), HitPoints: playv1.MonsterHitPoints(9)},
		"a creature twice": {Monsters: groups(goblin, 1, goblin, 2)},
		"a name of 31":     {Monsters: []*playv1.MonsterGroup{{CreatureKey: goblin, Name: strings.Repeat("x", 31)}}},
	} {
		if _, err := m.master.encounters.SaveBattleEncounter(t.Context(), connect.NewRequest(&playv1.SaveBattleEncounterRequest{CampaignId: m.campaignID, MapPointId: ruins, Encounter: be})); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("SaveBattleEncounter(%s) = %v, want invalid_argument", name, err)
		}
	}
	// A map that is not the campaign's.
	if _, err := m.master.encounters.ListBattleEncounters(t.Context(), connect.NewRequest(&playv1.ListBattleEncountersRequest{CampaignId: m.campaignID, MapId: other.mapID})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("ListBattleEncounters(another campaign's map) = %v, want not_found", err)
	}
}

// monstersLine is the master's line of the monsters a start put in, or nil.
func (m *mirathel) monstersLine(t *testing.T, e *playv1.Encounter) *playv1.CombatLogEntry {
	t.Helper()
	var line *playv1.CombatLogEntry
	for _, r := range m.log(t, m.master, e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_MONSTERS_ADDED {
				line = en
			}
		}
	}
	return line
}

// startFrom starts the combat of the master from what a battle point keeps, as the app does after
// "Começar este combate": the monsters, the hit points and the hidden switch of the saved encounter,
// and the point itself for a combat on its map.
func (m *mirathel) startFrom(t *testing.T, key, point string, mode playv1.EncounterMode) (*playv1.StartEncounterResponse, error) {
	t.Helper()
	saved := m.read(t, point).GetEncounter()
	res, err := m.master.combat.StartEncounter(t.Context(), connect.NewRequest(&playv1.StartEncounterRequest{
		CampaignId: m.campaignID, IdempotencyKey: key, Name: "Emboscada na ponte", MapPointId: pointIfGrid(point, mode), Mode: mode,
		Monsters: saved.GetMonsters(), MonsterHitPoints: saved.GetHitPoints(), MonstersHidden: saved.Hidden,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func pointIfGrid(point string, mode playv1.EncounterMode) string {
	if mode == playv1.EncounterMode_ENCOUNTER_MODE_THEATRE {
		return "" // a combat without a map takes no battle point
	}
	return point
}

// TestMR043_BeginningThisCombatPutsTheSavedMonstersIn: "Começar este combate" (artboard 7). The monsters
// of the saved encounter join the combat in the same transaction as the start, through the same code as
// AddMonsters: NPCs numbered together (Bugbear 1 and 2, Hobgoblin 1 to 4, Goblin 1 to 6), hidden, with
// the average hit points and an initiative of their own, in a combat on the point's map. The same key
// again changes nothing.
func TestMR043_BeginningThisCombatPutsTheSavedMonstersIn(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	bridge := m.point(t, "Emboscada na ponte")
	m.save(t, bridge, &playv1.BattleEncounter{Monsters: artboardOne()})

	m.h.roller.queue(15, 3, 7) // the Ogro, the first Bugbear, the second Bugbear; the rest roll 10
	key := newKey()
	res, err := m.startFrom(t, key, bridge, playv1.EncounterMode_ENCOUNTER_MODE_UNSPECIFIED)
	if err != nil {
		t.Fatalf("StartEncounter() error = %v", err)
	}
	e := res.GetEncounter()
	if e.GetMode() != playv1.EncounterMode_ENCOUNTER_MODE_GRID || e.GetMapId() != m.mapID || e.GetMapPointId() != bridge || len(e.GetCombatants()) != 17 {
		t.Fatalf("the combat = mode %v on %q, point %q, %d combatants; want GRID on the map, the point and 17", e.GetMode(), e.GetMapId(), e.GetMapPointId(), len(e.GetCombatants()))
	}
	for _, want := range []struct {
		label     string
		hp, xp    int32
		face      int32
		bonusSign int32
	}{
		{"Ogro", 59, 450, 15, -1},
		{"Bugbear 1", 27, 200, 3, 0},
		{"Bugbear 2", 27, 200, 7, 0},
		{"Hobgoblin 1", 11, 100, 10, 0},
		{"Hobgoblin 4", 11, 100, 10, 0},
		{"Goblin 1", 7, 50, 10, 0},
		{"Goblin 6", 7, 50, 10, 0},
	} {
		c := byLabel(t, e, want.label)
		if c.GetKind() != playv1.CombatantKind_COMBATANT_KIND_NPC || !c.GetHidden() || c.GetHitPointsMax() != want.hp || c.GetHitPointsCurrent() != want.hp || c.GetXpValue() != want.xp || c.GetPlaced() {
			t.Errorf("%s = %v; want a hidden NPC with %d hit points, %d XP, no square", want.label, c, want.hp, want.xp)
		}
		if c.GetInitiative() == 0 || c.GetInitiativeFace() != want.face {
			t.Errorf("%s: initiative %d with d20 %d, want a roll of its own (d20 %d)", want.label, c.GetInitiative(), c.GetInitiativeFace(), want.face)
		}
		if c.GetBestiaryCreatureKey() == "" {
			t.Errorf("%s has no creature key for the master", want.label)
		}
	}
	for _, label := range []string{"Toren", "Pensantus", "Brisa", "Sálvia"} {
		if c := byLabel(t, e, label); c.GetKind() != playv1.CombatantKind_COMBATANT_KIND_PLAYER || c.GetInitiative() != 0 && c.GetInitiativeFace() != 0 {
			t.Errorf("%s = %v, want a player with no initiative yet", label, c)
		}
	}
	// The master's line of the monsters, from before the combat.
	line := m.monstersLine(t, e)
	if line == nil || len(line.GetMonsters()) != 13 || line.GetMonsters()[0].GetLabel() != "Ogro" || line.GetMonsters()[0].GetHitPoints() != 59 {
		t.Errorf("the monsters' line = %v, want 13 monsters, the Ogro first with 59", line)
	}

	// The same key again: the same combat, nothing added, no dice rolled.
	m.h.roller.queue(1, 1, 1)
	again, err := m.startFrom(t, key, bridge, playv1.EncounterMode_ENCOUNTER_MODE_UNSPECIFIED)
	if err != nil || again.GetEncounter().GetId() != e.GetId() || len(again.GetEncounter().GetCombatants()) != 17 {
		t.Errorf("the retry = %v, %v; want the same combat with 17 combatants", again.GetEncounter().GetId(), err)
	}
	// And a second combat is refused, with the monsters of the first not doubled.
	_, err = m.startFrom(t, newKey(), bridge, playv1.EncounterMode_ENCOUNTER_MODE_UNSPECIFIED)
	wantBlockedBy(t, "a second start", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ENCOUNTER_ALREADY_OPEN)
	if n := len(m.get(t, m.master).GetCombatants()); n != 17 {
		t.Errorf("the combat has %d combatants after a refused start, want 17", n)
	}

	// The monsters are the master's until he reveals them: a player reads only the party.
	for who, u := range map[string]*user{"Caio": m.caio, "Dani": m.dani} {
		seen := m.get(t, u)
		raw, _ := protojson.Marshal(seen)
		logs, _ := protojson.Marshal(m.log(t, u, seen))
		for _, banned := range []string{"Ogro", "Bugbear", "Goblin", "monster:", "bestiary", "hitPoints"} {
			if strings.Contains(string(raw), banned) || strings.Contains(string(logs), banned) {
				t.Errorf("%s reads %q: %s %s", who, banned, raw, logs)
			}
		}
		if len(seen.GetCombatants()) != 4 {
			t.Errorf("%s reads %d combatants, want the 4 players", who, len(seen.GetCombatants()))
		}
	}
}

// TestMR043_BeginningThisCombatWorksInTheatreAndWithRolledHitPoints: the same monsters in a combat
// without a map (no point, no square), and with the hit points rolled: the dice of each monster before
// the d20 of its initiative, in the master's line.
func TestMR043_BeginningThisCombatWorksInTheatreAndWithRolledHitPoints(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	bridge := m.point(t, "Emboscada na ponte")
	m.save(t, bridge, &playv1.BattleEncounter{
		Monsters:  []*playv1.MonsterGroup{{CreatureKey: bandit, Count: 2, Name: "Salteador"}, {CreatureKey: ogre, Count: 1}},
		HitPoints: playv1.MonsterHitPoints_MONSTER_HIT_POINTS_ROLLED, Hidden: new(false),
	})
	// The hit dice of every monster first (the bandits' 2d8+2: 3+4 and 8+8; the Ogro's 7d10+21: 7 tens),
	// then the d20 of each initiative.
	m.h.roller.queue(3, 4, 8, 8, 10, 10, 10, 10, 10, 10, 10, 12, 5, 9)
	res, err := m.startFrom(t, newKey(), bridge, playv1.EncounterMode_ENCOUNTER_MODE_THEATRE)
	if err != nil {
		t.Fatalf("StartEncounter(theatre) error = %v", err)
	}
	e := res.GetEncounter()
	wantNoSquares(t, "a theatre combat with monsters", e)
	if e.GetMapPointId() != "" || len(e.GetCombatants()) != 7 {
		t.Fatalf("the combat = point %q with %d combatants, want none and 7", e.GetMapPointId(), len(e.GetCombatants()))
	}
	s1, s2, o := byLabel(t, e, "Salteador 1"), byLabel(t, e, "Salteador 2"), byLabel(t, e, "Ogro")
	if s1.GetHitPointsMax() != 9 || s2.GetHitPointsMax() != 18 || o.GetHitPointsMax() != 7*10+21 {
		t.Errorf("hit points %d, %d and %d, want 9 (3+4+2), 18 (8+8+2) and 91", s1.GetHitPointsMax(), s2.GetHitPointsMax(), o.GetHitPointsMax())
	}
	if s1.GetHidden() || s2.GetHidden() || o.GetHidden() {
		t.Error("the monsters are hidden, the save said they start in the open")
	}
	if s1.GetInitiativeFace() != 12 || s2.GetInitiativeFace() != 5 || o.GetInitiativeFace() != 9 {
		t.Errorf("d20s %d, %d and %d, want 12, 5 and 9: one for each, after the hit dice", s1.GetInitiativeFace(), s2.GetInitiativeFace(), o.GetInitiativeFace())
	}
	line := m.monstersLine(t, e)
	if line == nil || len(line.GetMonsters()) != 3 || !line.GetMonsters()[0].GetRolled() || line.GetMonsters()[0].GetDice() != "2d8+2" || line.GetMonsters()[0].GetLabel() != "Salteador 1" {
		t.Errorf("the monsters' line = %v, want the rolled 2d8+2 of Salteador 1 first", line)
	}
}

// TestMR043_StartingWithMonstersRefusals: what a start with monsters cannot be.
func TestMR043_StartingWithMonstersRefusals(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	for name, edit := range map[string]func(*playv1.StartEncounterRequest){
		"a creature that is not in the SRD": func(r *playv1.StartEncounterRequest) { r.Monsters = groups("monster:nope", 1) },
		"a creature twice":                  func(r *playv1.StartEncounterRequest) { r.Monsters = groups(goblin, 1, goblin, 1) },
		"41 of one":                         func(r *playv1.StartEncounterRequest) { r.Monsters = groups(goblin, 41) },
		"37 with the party of 4 (41)":       func(r *playv1.StartEncounterRequest) { r.Monsters = groups(goblin, 37) },
		"a name of 31": func(r *playv1.StartEncounterRequest) {
			r.Monsters = []*playv1.MonsterGroup{{CreatureKey: goblin, Name: strings.Repeat("x", 31)}}
		},
		"hit points of 9": func(r *playv1.StartEncounterRequest) {
			r.Monsters, r.MonsterHitPoints = groups(goblin, 1), playv1.MonsterHitPoints(9)
		},
		"21 groups": func(r *playv1.StartEncounterRequest) {
			for i := range 21 {
				r.Monsters = append(r.Monsters, &playv1.MonsterGroup{CreatureKey: []string{goblin, ogre, bugbear}[i%3]})
			}
		},
		"nobody at all": func(*playv1.StartEncounterRequest) {},
	} {
		req := &playv1.StartEncounterRequest{CampaignId: m.campaignID, IdempotencyKey: newKey(), Name: "x"}
		edit(req)
		_, err := m.master.combat.StartEncounter(t.Context(), connect.NewRequest(req))
		if strings.Contains(name, "(41)") {
			// The cap is typed, so the app can say it.
			wantBlockedBy(t, "StartEncounter("+name+")", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TOO_MANY_COMBATANTS)
			continue
		}
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("StartEncounter(%s) = %v, want invalid_argument", name, err)
		}
	}
	// 36 monsters and the party of 4 are 40: allowed.
	if _, err := m.master.combat.StartEncounter(t.Context(), connect.NewRequest(&playv1.StartEncounterRequest{
		CampaignId: m.campaignID, IdempotencyKey: newKey(), Name: "Enxame", Monsters: groups(goblin, 36),
	})); err != nil {
		t.Errorf("StartEncounter(36 goblins and the party) error = %v", err)
	}
}

// TestRN10_PlayersNeverGetTheBuilderOrTheSavedEncounter: the builder and what a point keeps are the
// master's. Every call answers a player not_found, and what a player's stream, its combat and its log
// carry has nothing of it, before and after the combat starts.
func TestRN10_PlayersNeverGetTheBuilderOrTheSavedEncounter(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	bridge := m.point(t, "Emboscada na ponte")
	w := m.caio.watch(t, m.campaignID)
	w.ready(t)

	m.save(t, bridge, &playv1.BattleEncounter{Monsters: artboardOne()})
	m.read(t, bridge)
	if _, err := m.master.encounters.ClearBattleEncounter(t.Context(), connect.NewRequest(&playv1.ClearBattleEncounterRequest{CampaignId: m.campaignID, MapPointId: bridge})); err != nil {
		t.Fatalf("ClearBattleEncounter() error = %v", err)
	}
	m.save(t, bridge, &playv1.BattleEncounter{Monsters: artboardOne()})
	// Saving, reading and clearing publish nothing at all to the table's stream: the
	// marker comes first on it, with nothing before.
	m.master.markCurrentMap(t, m.campaignID, m.mapID, w)
	for _, ev := range w.beforeMarker(t) {
		t.Errorf("a player's stream got %v after the master saved an encounter", ev)
	}

	if _, err := m.startFrom(t, newKey(), bridge, playv1.EncounterMode_ENCOUNTER_MODE_UNSPECIFIED); err != nil {
		t.Fatalf("StartEncounter() error = %v", err)
	}
	var all []*playv1.WatchGameSessionResponse
	deadline := time.After(10 * time.Second)
	for len(all) < 1 {
		select {
		case ev := <-w.events:
			if ev.GetHeartbeat() == nil {
				all = append(all, ev)
			}
		case <-deadline:
			t.Fatal("no event after the start")
		}
	}
	text := asJSONAll(t, all)
	for _, banned := range []string{"Ogro", "Bugbear", "Goblin", "monster:", "creature", "xp", bridge} {
		if strings.Contains(text, banned) {
			t.Errorf("a player's stream has %q: %s", banned, text)
		}
	}
}

// TestMR043_TheBuilderAuthorizationMatrix: every call is the master's. Signed out is
// unauthenticated; a player, a pending member and an outsider get not_found, so nothing says the builder
// or a saved encounter exists (RN-10).
func TestMR043_TheBuilderAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	pending := m.h.newUser("Pendente")
	m.h.joinPending(m.master, m.campaignID, pending)
	outsider := m.h.newUser("Fora")
	anonymous := m.h.anonymous()
	bridge := m.point(t, "Emboscada na ponte")
	m.save(t, bridge, &playv1.BattleEncounter{Monsters: groups(goblin, 2)})

	campaign := m.campaignID
	calls := map[string]func(u *user) error{
		"EvaluateEncounter": func(u *user) error {
			_, err := u.encounters.EvaluateEncounter(t.Context(), connect.NewRequest(&playv1.EvaluateEncounterRequest{CampaignId: campaign, Entries: groups(goblin, 1)}))
			return err
		},
		"GenerateEncounter": func(u *user) error {
			_, err := u.encounters.GenerateEncounter(t.Context(), connect.NewRequest(&playv1.GenerateEncounterRequest{CampaignId: campaign, Seed: 1}))
			return err
		},
		"ListEncounterSwaps": func(u *user) error {
			_, err := u.encounters.ListEncounterSwaps(t.Context(), connect.NewRequest(&playv1.ListEncounterSwapsRequest{CampaignId: campaign, CreatureKey: thug}))
			return err
		},
		"SaveBattleEncounter": func(u *user) error {
			_, err := u.encounters.SaveBattleEncounter(t.Context(), connect.NewRequest(&playv1.SaveBattleEncounterRequest{CampaignId: campaign, MapPointId: bridge, Encounter: &playv1.BattleEncounter{Monsters: groups(goblin, 2)}}))
			return err
		},
		"GetBattleEncounter": func(u *user) error {
			_, err := u.encounters.GetBattleEncounter(t.Context(), connect.NewRequest(&playv1.GetBattleEncounterRequest{CampaignId: campaign, MapPointId: bridge}))
			return err
		},
		"ClearBattleEncounter": func(u *user) error {
			_, err := u.encounters.ClearBattleEncounter(t.Context(), connect.NewRequest(&playv1.ClearBattleEncounterRequest{CampaignId: campaign, MapPointId: newKey()}))
			return err
		},
		"ListBattleEncounters": func(u *user) error {
			_, err := u.encounters.ListBattleEncounters(t.Context(), connect.NewRequest(&playv1.ListBattleEncountersRequest{CampaignId: campaign, MapId: m.mapID}))
			return err
		},
	}
	methods := playv1.File_meurpg_play_v1_encounters_proto.Services().ByName("EncounterService").Methods()
	if len(calls) != methods.Len() {
		t.Errorf("the matrix covers %d methods, the service has %d", len(calls), methods.Len())
	}
	for name, call := range calls {
		if err := call(anonymous); connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("%s signed out: %v, want unauthenticated", name, err)
		}
		for who, u := range map[string]*user{"outsider": outsider, "pending": pending, "Caio (a player)": m.caio, "Dani (a player)": m.dani} {
			if err := call(u); connect.CodeOf(err) != connect.CodeNotFound {
				t.Errorf("%s as %s: %v, want not_found", name, who, err)
			}
		}
		if name != "ClearBattleEncounter" { // that one names a point of nobody's: not_found for the master too
			if err := call(m.master); err != nil {
				t.Errorf("%s as the master: %v", name, err)
			}
		}
	}
	if err := calls["ClearBattleEncounter"](m.master); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("ClearBattleEncounter(a point that is nobody's) = %v, want not_found", err)
	}
}

// TestMR043_AStartRetryIsCheckedAgainstTheRequest: the same key with another request is refused, as
// AddMonsters refuses it; the same request again still answers with the same combat (fix round 1).
func TestMR043_AStartRetryIsCheckedAgainstTheRequest(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	key := newKey()
	req := func(edit func(*playv1.StartEncounterRequest)) *playv1.StartEncounterRequest {
		r := &playv1.StartEncounterRequest{CampaignId: m.campaignID, IdempotencyKey: key, Name: "Emboscada", Monsters: groups(goblin, 3)}
		if edit != nil {
			edit(r)
		}
		return r
	}
	first, err := m.master.combat.StartEncounter(t.Context(), connect.NewRequest(req(nil)))
	if err != nil {
		t.Fatalf("StartEncounter() error = %v", err)
	}
	again, err := m.master.combat.StartEncounter(t.Context(), connect.NewRequest(req(nil)))
	if err != nil || again.Msg.GetEncounter().GetId() != first.Msg.GetEncounter().GetId() || len(again.Msg.GetEncounter().GetCombatants()) != 7 {
		t.Fatalf("the same request again = %v, %v; want the same combat", again, err)
	}
	for name, edit := range map[string]func(*playv1.StartEncounterRequest){
		"other monsters":   func(r *playv1.StartEncounterRequest) { r.Monsters = groups(goblin, 4) },
		"another creature": func(r *playv1.StartEncounterRequest) { r.Monsters = groups(ogre, 3) },
		"other hit points": func(r *playv1.StartEncounterRequest) {
			r.MonsterHitPoints = playv1.MonsterHitPoints_MONSTER_HIT_POINTS_ROLLED
		},
		"revealed monsters": func(r *playv1.StartEncounterRequest) { r.MonstersHidden = new(false) },
		"other participants": func(r *playv1.StartEncounterRequest) {
			r.Participants = []*playv1.Participant{{CharacterId: m.goblin.GetId()}}
		},
		"another mode":       func(r *playv1.StartEncounterRequest) { r.Mode = playv1.EncounterMode_ENCOUNTER_MODE_THEATRE },
		"another name":       func(r *playv1.StartEncounterRequest) { r.Name = "Outra" },
		"no monsters at all": func(r *playv1.StartEncounterRequest) { r.Monsters = nil },
	} {
		if _, err := m.master.combat.StartEncounter(t.Context(), connect.NewRequest(req(edit))); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("a retry with %s = %v, want invalid_argument", name, err)
		}
	}
	if n := len(m.get(t, m.master).GetCombatants()); n != 7 {
		t.Errorf("the combat has %d combatants after the retries, want 7", n)
	}
}

// TestMR043_AStartWithMonstersAndNpcParticipants: explicit NPC participants and monsters join together, the
// party too, and every label is its own.
func TestMR043_AStartWithMonstersAndNpcParticipants(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	res, err := m.master.combat.StartEncounter(t.Context(), connect.NewRequest(&playv1.StartEncounterRequest{
		CampaignId: m.campaignID, IdempotencyKey: newKey(), Name: "Ponte",
		Participants: []*playv1.Participant{{CharacterId: m.goblin.GetId(), Count: 2}, {CharacterId: m.capitao.GetId()}},
		Monsters:     groups(bandit, 3), MonsterHitPoints: playv1.MonsterHitPoints_MONSTER_HIT_POINTS_AVERAGE,
	}))
	if err != nil {
		t.Fatalf("StartEncounter() error = %v", err)
	}
	e := res.Msg.GetEncounter()
	// 4 players, 2 goblins, the Capitão and 3 bandits.
	if len(e.GetCombatants()) != 10 {
		t.Fatalf("combatants = %v, want 10", labels(e))
	}
	for _, label := range []string{"Goblin 1", "Goblin 2", "Capitão Goblin", "Bandido 1", "Bandido 3", "Toren", "Sálvia"} {
		byLabel(t, e, label)
	}
	if byLabel(t, e, "Bandido 1").GetHitPointsMax() != 11 || byLabel(t, e, "Goblin 1").GetHitPointsMax() != 7 {
		t.Error("the monsters and the NPCs keep their own hit points")
	}
}

// TestMR043_TheFortyCountsThePartysCreatures: a familiar joins the combat with its owner, so it takes room:
// TOO_MANY and the start count it. Four players and a familiar are 5 combatants: 35 monsters fit, 36 do not.
func TestMR043_TheFortyCountsThePartysCreatures(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	m.give(t, m.toren, "monster:bat", "Morcego")
	warns := func(n int) bool {
		return slices.Contains(m.evaluate(t, groups(goblin, n)).GetWarnings(), playv1.EncounterWarning_ENCOUNTER_WARNING_TOO_MANY)
	}
	if warns(35) || !warns(36) {
		t.Errorf("TOO_MANY at 35 monsters = %v, at 36 = %v; want false and true (4 players and a familiar)", warns(35), warns(36))
	}
	start := func(n int) error {
		_, err := m.master.combat.StartEncounter(t.Context(), connect.NewRequest(&playv1.StartEncounterRequest{CampaignId: m.campaignID, IdempotencyKey: newKey(), Name: "x", Monsters: groups(goblin, n)}))
		return err
	}
	// Typed, with the party's familiar counted, so the app can say it in words.
	wantBlockedBy(t, "StartEncounter(36 goblins, a party of 5)", start(36), playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TOO_MANY_COMBATANTS)
	if err := start(35); err != nil {
		t.Errorf("StartEncounter(35 goblins, a party of 5) error = %v", err)
	}
	res, err := m.generate(t, func(r *playv1.GenerateEncounterRequest) { r.CreatureType = "" })
	if err != nil || slices.Contains(res.GetEvaluation().GetWarnings(), playv1.EncounterWarning_ENCOUNTER_WARNING_TOO_MANY) {
		t.Errorf("a generated encounter = %v, %v; want no TOO_MANY", res, err)
	}
}

// TestMR043_AGeneratedEncounterNeverFillsTheCombat: with 20 player characters (level 5) the combat of 40 has
// room for 20 creatures, and no seed draws more or a TOO_MANY encounter.
func TestMR043_AGeneratedEncounterNeverFillsTheCombat(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Samuel")
	campaign := h.newCampaign(master, "Exército")
	scores := &rulesv1.AbilityScores{Strength: 14, Dexterity: 12, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}
	for i := range 20 {
		u := h.newUser("J" + string(rune('A'+i)))
		h.join(master, campaign, u)
		u.hero(t, campaign, "H"+string(rune('A'+i)), "class:fighter", "race:human", 5, scores, []string{battleaxe}, nil)
	}
	for seed := uint32(1); seed <= 30; seed++ {
		res, err := master.encounters.GenerateEncounter(t.Context(), connect.NewRequest(&playv1.GenerateEncounterRequest{CampaignId: campaign, Band: playv1.EncounterBand_ENCOUNTER_BAND_LOW, Seed: seed}))
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		ev := res.Msg.GetEvaluation()
		if ev.GetCreatureCount() > 20 || slices.Contains(ev.GetWarnings(), playv1.EncounterWarning_ENCOUNTER_WARNING_TOO_MANY) {
			t.Fatalf("seed %d: %d creatures for a party of 20, warnings %v", seed, ev.GetCreatureCount(), ev.GetWarnings())
		}
	}
}

// TestMR043_ASavedEncounterWithAGoneCreature: a creature that is no longer an SRD creature (a content change)
// leaves the encounter readable, marked; saving it again is refused with the typed reason, never unavailable.
func TestMR043_ASavedEncounterWithAGoneCreature(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	bridge := m.point(t, "Emboscada na ponte")
	if _, err := m.h.pool.Exec(t.Context(),
		`INSERT INTO battle_encounters (map_point_id, campaign_id, map_id, encounter, created_at, updated_at)
		 VALUES ($1, $2, $3, '{"monsters":[{"creatureKey":"monster:ogre","count":1},{"creatureKey":"monster:gone","count":2}],"hitPoints":"MONSTER_HIT_POINTS_AVERAGE","hidden":true}', now(), now())`,
		bridge, m.campaignID, m.mapID); err != nil {
		t.Fatalf("insert the saved encounter: %v", err)
	}
	got := m.read(t, bridge)
	if len(got.GetEncounter().GetMonsters()) != 2 || len(got.GetUnknownCreatureKeys()) != 1 || got.GetUnknownCreatureKeys()[0] != "monster:gone" ||
		!slices.Contains(got.GetEvaluation().GetWarnings(), playv1.EncounterWarning_ENCOUNTER_WARNING_UNKNOWN_CREATURE) || got.GetEvaluation().GetTotalXp() != 450 {
		t.Fatalf("read = %v", got)
	}
	_, err := m.master.encounters.SaveBattleEncounter(t.Context(), connect.NewRequest(&playv1.SaveBattleEncounterRequest{CampaignId: m.campaignID, MapPointId: bridge, Encounter: got.GetEncounter()}))
	wantBuildBlocked(t, "SaveBattleEncounter(a gone creature)", err, playv1.EncounterBuildBlockedReason_ENCOUNTER_BUILD_BLOCKED_REASON_UNKNOWN_CREATURE)
	m.save(t, bridge, &playv1.BattleEncounter{Monsters: groups(ogre, 1)}) // taking it out fixes it
	if g := m.read(t, bridge); len(g.GetUnknownCreatureKeys()) != 0 || len(g.GetEvaluation().GetWarnings()) != 0 {
		t.Errorf("after the fix: %v", g)
	}
}

// TestMR043_TheSavedEncounterFollowsItsPoint: deleting the point or the map takes the row (cascade); a point
// that stopped being a battle point is no longer listed, but the master can still clear it.
func TestMR043_TheSavedEncounterFollowsItsPoint(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	count := func() int {
		var n int
		if err := m.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM battle_encounters WHERE campaign_id = $1`, m.campaignID).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := m.h.pool.Exec(t.Context(), q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	listed := func() int {
		res, err := m.master.encounters.ListBattleEncounters(t.Context(), connect.NewRequest(&playv1.ListBattleEncountersRequest{CampaignId: m.campaignID, MapId: m.mapID}))
		if err != nil {
			t.Fatalf("ListBattleEncounters() error = %v", err)
		}
		return len(res.Msg.GetEncounters())
	}
	a, b := m.point(t, "A"), m.point(t, "B")
	m.save(t, a, &playv1.BattleEncounter{Monsters: groups(goblin, 1)})
	m.save(t, b, &playv1.BattleEncounter{Monsters: groups(goblin, 2)})
	if count() != 2 || listed() != 2 {
		t.Fatalf("saved %d, listed %d, want 2 and 2", count(), listed())
	}
	exec(`DELETE FROM map_points WHERE id = $1`, a)
	if count() != 1 {
		t.Errorf("deleting the point left %d rows, want 1", count())
	}
	exec(`UPDATE map_points SET kind = 'scene' WHERE id = $1`, b) // B stops being a battle point
	if listed() != 0 || count() != 1 {
		t.Errorf("a point that is a scene now: listed %d, rows %d; want 0 and 1", listed(), count())
	}
	if _, err := m.master.encounters.ClearBattleEncounter(t.Context(), connect.NewRequest(&playv1.ClearBattleEncounterRequest{CampaignId: m.campaignID, MapPointId: b})); err != nil || count() != 0 {
		t.Errorf("clearing the old battle point = %v, rows %d; want ok and 0", err, count())
	}
	if _, err := m.master.encounters.ClearBattleEncounter(t.Context(), connect.NewRequest(&playv1.ClearBattleEncounterRequest{CampaignId: m.campaignID, MapPointId: b})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("clearing a scene point with no row = %v, want not_found", err)
	}
	c := m.point(t, "C")
	m.save(t, c, &playv1.BattleEncounter{Monsters: groups(goblin, 1)})
	exec(`DELETE FROM maps WHERE id = $1`, m.mapID) // deleting the map takes its rows too
	if count() != 0 {
		t.Errorf("deleting the map left %d rows, want 0", count())
	}
}

// TestMR043_SavesAtOnceOnOnePoint: concurrent saves of different encounters on one point all succeed and leave
// one row, one of them whole.
func TestMR043_SavesAtOnceOnOnePoint(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 6)
	m := newMirathel(t)
	bridge := m.point(t, "Emboscada na ponte")
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	start := dbtest.NewBarrier(6)
	for i := range 6 {
		wg.Go(func() {
			start.Wait()
			_, err := m.master.encounters.SaveBattleEncounter(t.Context(), connect.NewRequest(&playv1.SaveBattleEncounterRequest{
				CampaignId: m.campaignID, MapPointId: bridge, Encounter: &playv1.BattleEncounter{Monsters: groups(goblin, i+1)},
			}))
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("a concurrent save failed: %v", err)
		}
	}
	got := m.read(t, bridge).GetEncounter()
	if len(got.GetMonsters()) != 1 || got.GetMonsters()[0].GetCount() < 1 || got.GetMonsters()[0].GetCount() > 6 {
		t.Errorf("the point keeps %v, want one of the six saves", got)
	}
	var rows int
	if err := m.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM battle_encounters WHERE map_point_id = $1`, bridge).Scan(&rows); err != nil || rows != 1 {
		t.Errorf("rows = %d, %v; want 1", rows, err)
	}
}

// TestRN10_TheMapsModuleNeverReadsTheSavedEncounter: what serves a player's map and points is the maps
// module (the harness does not mount it, so the guarantee is checked in its source); it must not touch
// battle_encounters, the master's table.
func TestRN10_TheMapsModuleNeverReadsTheSavedEncounter(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("../maps/*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no maps files: %v", err)
	}
	more, _ := filepath.Glob("../maps/queries.sql")
	for _, f := range append(files, more...) {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "battle_encounters") {
			t.Errorf("%s reads battle_encounters, which only the master's encounter builder may", f)
		}
	}
}

// TestMR578_ATableCreatureIsRefusedByTheEncounterTools: the evaluator, the save and the swaps read the SRD's
// challenge ratings, so a table creature's key is refused as an unknown creature (never an internal error), and a
// saved encounter that holds one is read back with the key reported and the rest measured.
func TestMR578_ATableCreatureIsRefusedByTheEncounterTools(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	ctx := t.Context()
	if _, err := m.master.table.CreateTableEntry(ctx, connect.NewRequest(&rulesv1.CreateTableEntryRequest{
		CampaignId: m.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableCreature{TableCreature: &rulesv1.TableCreature{
			NamePt: "Guarda Ossuda", BaseKey: "monster:skeleton", HitPoints: 30,
		}},
	})); err != nil {
		t.Fatalf("CreateTableEntry(creature) error = %v", err)
	}
	wantRefused := func(what string, err error) {
		t.Helper()
		if connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Errorf("%s with a table creature: error = %v, want failed_precondition (an unknown creature)", what, err)
		}
	}
	_, err := m.master.encounters.EvaluateEncounter(ctx, connect.NewRequest(&playv1.EvaluateEncounterRequest{CampaignId: m.campaignID, Entries: groups(tableSkeletonKey, 1)}))
	wantRefused("EvaluateEncounter", err)
	point := m.point(t, "Ossuário")
	_, err = m.master.encounters.SaveBattleEncounter(ctx, connect.NewRequest(&playv1.SaveBattleEncounterRequest{CampaignId: m.campaignID, MapPointId: point, Encounter: &playv1.BattleEncounter{Monsters: groups(tableSkeletonKey, 1)}}))
	wantRefused("SaveBattleEncounter", err)
	_, err = m.master.encounters.ListEncounterSwaps(ctx, connect.NewRequest(&playv1.ListEncounterSwapsRequest{CampaignId: m.campaignID, CreatureKey: tableSkeletonKey}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("ListEncounterSwaps with a table creature: error = %v, want invalid_argument", err)
	}

	// A saved encounter whose key is a table key (written by hand): the read does not fail.
	m.save(t, point, &playv1.BattleEncounter{Monsters: groups(goblin, 2)})
	if _, err := m.h.pool.Exec(ctx, `UPDATE battle_encounters SET encounter = replace(encounter::text, $1, $2)::jsonb WHERE map_point_id = $3`, goblin, tableSkeletonKey, point); err != nil {
		t.Fatalf("rewrite the saved encounter: %v", err)
	}
	got := m.read(t, point)
	if len(got.GetUnknownCreatureKeys()) != 1 || got.GetUnknownCreatureKeys()[0] != tableSkeletonKey {
		t.Errorf("GetBattleEncounter unknown keys = %v, want the table creature's key", got.GetUnknownCreatureKeys())
	}
}
