package play

import (
	"regexp"
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

// "Pôr no combate" (MR-042, RN-29; Etapa 10, slice 10.9b): monsters of an SRD
// creature join a combat as NPC combatants. These tests need the database
// (MEURPG_TEST_DATABASE_URL). The creature is the Bandido (monster:bandit): CA 12,
// 11 PV (2d8+2), Destreza 12 (+1), ND 1/8 (25 XP), a cimitarra at +3 for 1d6+1.

const bandit = "monster:bandit"

// addMonsters calls AddMonsters as the master; edit changes the request.
func (a *armed) addMonsters(t *testing.T, e *playv1.Encounter, key string, edit func(*playv1.AddMonstersRequest)) (*playv1.AddMonstersResponse, error) {
	t.Helper()
	req := &playv1.AddMonstersRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: key, CreatureKey: bandit, Count: 3}
	if edit != nil {
		edit(req)
	}
	res, err := a.master.combat.AddMonsters(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (a *armed) mustAddMonsters(t *testing.T, e *playv1.Encounter, edit func(*playv1.AddMonstersRequest)) *playv1.AddMonstersResponse {
	t.Helper()
	res, err := a.addMonsters(t, e, newKey(), edit)
	if err != nil {
		t.Fatalf("AddMonsters() error = %v", err)
	}
	return res
}

// monsterSetup is a combat in SETUP with the party and a revealed goblin, on a
// map or without one, ready for the master to add monsters.
func (a *armed) monsterSetup(t *testing.T, theatre bool) *playv1.Encounter {
	t.Helper()
	return a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{1},
		players: map[string]int32{"Toren": 10, "Pensantus": 5, "Brisa": 1},
		setup:   true, theatre: theatre,
	})
}

func (a *armed) begin(t *testing.T, e *playv1.Encounter) *playv1.Encounter {
	t.Helper()
	res, err := a.master.combat.BeginCombat(t.Context(), connect.NewRequest(&playv1.BeginCombatRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()}))
	if err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	return res.Msg.GetEncounter()
}

// masterNpcs counts the NPCs in the master's list of characters.
func (a *armed) masterNpcs(t *testing.T) int {
	t.Helper()
	res, err := a.master.characters.ListCharacters(t.Context(), connect.NewRequest(&charactersv1.ListCharactersRequest{CampaignId: a.campaignID}))
	if err != nil {
		t.Fatalf("ListCharacters() error = %v", err)
	}
	n := 0
	for _, c := range res.Msg.GetCharacters() {
		if c.GetKind() != charactersv1.CharacterKind_CHARACTER_KIND_PLAYER {
			n++
		}
	}
	return n
}

// TestMR042_ThreeBanditsJoinTheCombat: three Bandidos named "Bandido 1" to "Bandido 3",
// the average hit points, an initiative rolled for each with the Dexterity modifier
// (d20 10, 15 and 4, plus 1), hidden, with no square, and the NPCs of the master's
// list unchanged. The same key again adds nothing and answers with the same ones.
func TestMR042_ThreeBanditsJoinTheCombat(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.monsterSetup(t, false)
	before := a.masterNpcs(t)

	a.h.roller.queue(10, 15, 4)
	key := newKey()
	res, err := a.addMonsters(t, e, key, nil)
	if err != nil {
		t.Fatalf("AddMonsters() error = %v", err)
	}
	if len(res.GetCombatantIds()) != 3 {
		t.Fatalf("combatant_ids = %v, want 3", res.GetCombatantIds())
	}
	want := []struct {
		label   string
		initial int32
		face    int32
	}{{"Bandido 1", 11, 10}, {"Bandido 2", 16, 15}, {"Bandido 3", 5, 4}}
	for i, w := range want {
		c := byLabel(t, res.GetEncounter(), w.label)
		if c.GetId() != res.GetCombatantIds()[i] {
			t.Errorf("%s is %s, combatant_ids[%d] = %s", w.label, c.GetId(), i, res.GetCombatantIds()[i])
		}
		if c.GetKind() != playv1.CombatantKind_COMBATANT_KIND_NPC || !c.GetHidden() || c.GetHitPointsCurrent() != 11 || c.GetHitPointsMax() != 11 || c.GetArmorClass() != 12 {
			t.Errorf("%s = %v, want a hidden NPC with 11 hit points and armor class 12", w.label, c)
		}
		if c.GetInitiative() != w.initial || c.GetInitiativeFace() != w.face || c.GetInitiativeBonus() != 1 {
			t.Errorf("%s: initiative %d (d20 %d, bonus %d), want %d (d20 %d, bonus 1)", w.label, c.GetInitiative(), c.GetInitiativeFace(), c.GetInitiativeBonus(), w.initial, w.face)
		}
		if c.GetPlaced() {
			t.Errorf("%s stands on a square, want none until the master places it", w.label)
		}
		if c.GetXpValue() != 25 {
			t.Errorf("%s gives %d XP, want 25 (ND 1/8)", w.label, c.GetXpValue())
		}
	}
	if n := len(res.GetEncounter().GetCombatants()); n != 7 { // Toren, Pensantus, Brisa, Goblin, three Bandidos
		t.Errorf("the combat has %d combatants, want 7", n)
	}
	if got := a.masterNpcs(t); got != before {
		t.Errorf("the master's list has %d NPCs, want the %d it had: the monsters' NPCs are not listed", got, before)
	}

	// The same key again: the same answer, nothing added, no dice rolled.
	a.h.roller.queue(20, 20, 20)
	again, err := a.addMonsters(t, e, key, nil)
	if err != nil {
		t.Fatalf("AddMonsters(again) error = %v", err)
	}
	if strings.Join(again.GetCombatantIds(), ",") != strings.Join(res.GetCombatantIds(), ",") || len(again.GetEncounter().GetCombatants()) != 7 {
		t.Errorf("the retry = %v with %d combatants, want the same ids and 7", again.GetCombatantIds(), len(again.GetEncounter().GetCombatants()))
	}
	// Another add keeps counting: "Bandido 4", and a name of the master's own.
	more := a.mustAddMonsters(t, res.GetEncounter(), func(r *playv1.AddMonstersRequest) { r.Count = 1 })
	byLabel(t, more.GetEncounter(), "Bandido 4")
	named := a.mustAddMonsters(t, more.GetEncounter(), func(r *playv1.AddMonstersRequest) { r.Count = 2; r.Name = "Salteador"; r.Hidden = new(false) })
	if c := byLabel(t, named.GetEncounter(), "Salteador 1"); c.GetHidden() {
		t.Errorf("Salteador 1 = %v, want it revealed when the request says so", c)
	}
	byLabel(t, named.GetEncounter(), "Salteador 2")
}

// TestMR042_MonstersRollTheirHitPoints: with ROLLED, each monster rolls the creature's
// hit dice (2d8+2), at least 1 and never more than the dice allow.
func TestMR042_MonstersRollTheirHitPoints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.monsterSetup(t, false)
	// The hit dice first (Bandido 1: 3 and 4, Bandido 2: 8 and 8), then the initiative.
	a.h.roller.queue(3, 4, 8, 8, 10, 15)
	res := a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) {
		r.Count = 2
		r.HitPoints = playv1.MonsterHitPoints_MONSTER_HIT_POINTS_ROLLED
	})
	for label, hp := range map[string]int32{"Bandido 1": 9, "Bandido 2": 18} {
		if c := byLabel(t, res.GetEncounter(), label); c.GetHitPointsCurrent() != hp || c.GetHitPointsMax() != hp {
			t.Errorf("%s has %d/%d hit points, want %d (2d8+2 rolled)", label, c.GetHitPointsCurrent(), c.GetHitPointsMax(), hp)
		}
	}
	if c := byLabel(t, res.GetEncounter(), "Bandido 1"); c.GetInitiative() != 11 {
		t.Errorf("Bandido 1's initiative = %d, want 11 (the d20 is rolled after the hit dice)", c.GetInitiative())
	}
}

// TestMR042_AddMonstersRefusals: what the master cannot ask for.
func TestMR042_AddMonstersRefusals(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.monsterSetup(t, false)
	for name, edit := range map[string]func(*playv1.AddMonstersRequest){
		"a creature that is not in the SRD": func(r *playv1.AddMonstersRequest) { r.CreatureKey = "monster:no-such-thing" },
		"a key of something else":           func(r *playv1.AddMonstersRequest) { r.CreatureKey = "spell:fireball" },
		"eleven":                            func(r *playv1.AddMonstersRequest) { r.Count = 11 },
		"a negative count":                  func(r *playv1.AddMonstersRequest) { r.Count = -1 },
		"a name of two lines":               func(r *playv1.AddMonstersRequest) { r.Name = "a\nb" },
		"a name that is too long":           func(r *playv1.AddMonstersRequest) { r.Name = strings.Repeat("x", 31) },
	} {
		if _, err := a.addMonsters(t, e, newKey(), edit); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("AddMonsters(%s) = %v, want invalid_argument", name, err)
		}
	}
	// A player cannot, and a combat that is full takes no more.
	req := &playv1.AddMonstersRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), CreatureKey: bandit, Count: 1}
	if _, err := a.caio.combat.AddMonsters(t.Context(), connect.NewRequest(req)); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("AddMonsters(as a player) = %v, want permission_denied", err)
	}
	// The cap is 40 combatants: 39 plus one is accepted, 40 plus one refused.
	for _, n := range []int32{9, 9, 9, 8, 1} { // 4 + 36 = 40 in the end
		a.mustAddMonsters(t, a.get(t, a.master), func(r *playv1.AddMonstersRequest) { r.Count = n })
	}
	_, capErr := a.addMonsters(t, a.get(t, a.master), newKey(), func(r *playv1.AddMonstersRequest) { r.Count = 1 })
	wantBlockedBy(t, "AddMonsters(past 40 combatants)", capErr, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TOO_MANY_COMBATANTS)
	// A combat of another campaign is not found.
	other := newArmed(t)
	foreign := other.monsterSetup(t, false)
	if _, err := a.addMonsters(t, foreign, newKey(), nil); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("AddMonsters(a combat of another campaign) = %v, want not_found", err)
	}
}

// TestMR042_AMonstersCriticalFollowsTheTablesRule: a Bandido's scimitar goes through
// RollAttack, so a natural 20 follows the table's critical rule (RN-24) like any
// NPC's, with each rule.
func TestMR042_AMonstersCriticalFollowsTheTablesRule(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	for _, tc := range []struct {
		name       string
		set        func()
		rule       playv1.CriticalDamageRule
		dice, kept int32
	}{
		{"doubled dice", func() { a.setRules(t, criticalIs(doubled)) }, playv1.CriticalDamageRule_CRITICAL_DAMAGE_RULE_DOUBLED_DICE, 2, 0},
		{"maximum plus a roll", func() { a.setRules(t, criticalIs(maxRoll)) }, playv1.CriticalDamageRule_CRITICAL_DAMAGE_RULE_MAX_PLUS_ROLL, 1, 6},
	} {
		tc.set()
		// Bandido 1 rolls a 20 for the initiative: it plays first. No map: no squares.
		e := a.monsterSetup(t, true)
		a.h.roller.queue(20)
		a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) { r.Count = 1 })
		e = a.begin(t, a.get(t, a.master))
		if e.GetCurrentCombatantId() != a.id(t, "Bandido") {
			t.Fatalf("%s: %s is on turn, want the Bandido", tc.name, e.GetCurrentCombatantId())
		}
		hit := a.mustAttack(t, a.master, e, "Bandido", banditScimitar, "Toren", d20(20))
		if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_CRITICAL_HIT {
			t.Fatalf("%s: the Bandido's roll = %v, want a critical hit", tc.name, hit.GetRoll())
		}
		wantCritical(t, tc.name+": the Bandido's cimitarra", hit.GetPendingDamage(), tc.rule, tc.dice, tc.kept)
		a.endEncounter(t, e)
		// A new combat for the next rule: the open one is ended above.
	}
}

// TestRN20_PlayersNeverReceiveAMonstersNumbers: of three Bandidos, one revealed and
// two hidden, the player's JSON (the combat and the log) has the revealed one by its
// state word only: no hit points, armor class, initiative or creature key, and
// nothing of the hidden ones, not even their ids or names.
func TestRN20_PlayersNeverReceiveAMonstersNumbers(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.monsterSetup(t, false)
	a.h.roller.queue(10, 15, 4)
	res := a.mustAddMonsters(t, e, nil)
	hidden := []*playv1.Combatant{byLabel(t, res.GetEncounter(), "Bandido 1"), byLabel(t, res.GetEncounter(), "Bandido 3")}
	if _, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Bandido 2"), IdempotencyKey: newKey(), Hidden: false,
	})); err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}
	e = a.begin(t, a.get(t, a.master))
	// The revealed one is hurt: the player reads "Muito ferido", not 4 of 11.
	if _, err := a.adjustHP(t, e, "Bandido 2", damageHP(6)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}

	for who, u := range map[string]*user{"Caio": a.caio, "Ana": a.ana} {
		seen := a.get(t, u)
		raw, err := protojson.Marshal(seen)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		logs, err := protojson.Marshal(a.log(t, u, seen))
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		for _, text := range []string{string(raw), string(logs)} {
			for _, h := range hidden {
				if strings.Contains(text, h.GetId()) || strings.Contains(text, h.GetLabel()) {
					t.Errorf("%s reads the hidden %s: %s", who, h.GetLabel(), text)
				}
			}
			for _, banned := range []string{"hitPoints", "armor", "bandit", `"hidden"`, "xpValue"} {
				if strings.Contains(text, banned) {
					t.Errorf("%s reads %q: %s", who, banned, text)
				}
			}
		}
		c := byLabel(t, seen, "Bandido 2")
		if c.GetState() != playv1.CombatantState_COMBATANT_STATE_BADLY_HURT || c.Initiative != nil || c.HitPointsCurrent != nil || c.ArmorClass != nil {
			t.Errorf("%s: Bandido 2 = %v, want only the word Muito ferido", who, c)
		}
	}
	// The revealed monster attacks Toren (it plays first: 16): the player's log line has
	// the result and no number of the monster's.
	if e.GetCurrentCombatantId() != a.id(t, "Bandido 2") {
		t.Fatalf("on turn = %s, want Bandido 2", e.GetCurrentCombatantId())
	}
	a.mustAttack(t, a.master, e, "Bandido 2", banditScimitar, "Toren", d20(15))
	for who, u := range map[string]*user{"Caio": a.caio, "Ana": a.ana} {
		logs, err := protojson.Marshal(a.log(t, u, a.get(t, u)))
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		if !strings.Contains(string(logs), "Bandido 2") {
			t.Errorf("%s's log has no line of Bandido 2's attack: %s", who, logs)
		}
		for _, banned := range []string{"targetArmorClass", "armor", "hitPoints", "bandit", "faces"} {
			if strings.Contains(string(logs), banned) {
				t.Errorf("%s's log has %q after the monster attacked: %s", who, banned, logs)
			}
		}
		if c := byLabel(t, a.get(t, u), "Bandido 2"); c.GetCharacterId() != "" {
			t.Errorf("%s reads the revealed monster's character %s, want none", who, c.GetCharacterId())
		}
	}
	// The master reads all of it.
	if c := byLabel(t, a.get(t, a.master), "Bandido 2"); c.GetHitPointsCurrent() != 5 || c.GetArmorClass() != 12 {
		t.Errorf("Bandido 2 for the master = %v, want 5 hit points and armor class 12", c)
	}
}

// TestMR042_MonstersGiveXPByTheirChallengeRating: three Bandidos (ND 1/8, 25 XP each)
// that fall give 75 XP at the end of the combat (the split among the party, 18 each
// with 3 left over, is progression's: TestMR016 and friends).
func TestMR042_MonstersGiveXPByTheirChallengeRating(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.monsterSetup(t, false)
	a.mustAddMonsters(t, e, nil)
	e = a.begin(t, a.get(t, a.master))
	for _, label := range []string{"Bandido 1", "Bandido 2", "Bandido 3"} {
		if _, err := a.adjustHP(t, e, label, damageHP(100)); err != nil {
			t.Fatalf("AdjustCombatantHitPoints(%s) error = %v", label, err)
		}
	}
	a.endEncounter(t, e)
	enc, err := a.h.svc.CampaignEncounter(t.Context(), nil, a.campaignID, e.GetId())
	if err != nil {
		t.Fatalf("CampaignEncounter() error = %v", err)
	}
	if !enc.Ended || enc.XP != 75 {
		t.Errorf("the ended combat = %+v, want ended with 75 XP (3 x 25)", enc)
	}
}

// TestMR042_MonstersInTheatreHaveNoSquare: in a combat without a map the monsters join
// the same way, with no square, and fight on the master's word.
func TestMR042_MonstersInTheatreHaveNoSquare(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.monsterSetup(t, true)
	res := a.mustAddMonsters(t, e, nil)
	for _, c := range res.GetEncounter().GetCombatants() {
		if c.GetPlaced() {
			t.Errorf("%s has a square in a combat without a map", c.GetLabel())
		}
	}
	// Added while the combat runs: the turn does not change.
	e = a.begin(t, a.get(t, a.master))
	on := e.GetCurrentCombatantId()
	more := a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) { r.Count = 2; r.Name = "Capanga" })
	if more.GetEncounter().GetCurrentCombatantId() != on {
		t.Errorf("the turn passed to %s when monsters joined a running combat, want %s to stay", more.GetEncounter().GetCurrentCombatantId(), on)
	}
	byLabel(t, more.GetEncounter(), "Capanga 2")
}

// monsterNpcs counts the NPCs the app keeps for monsters in the campaign.
func (a *armed) monsterNpcs(t *testing.T) int {
	t.Helper()
	var n int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM characters WHERE campaign_id = $1 AND sheet->'basic'->>'combat_only' = 'true'`, a.campaignID).Scan(&n); err != nil {
		t.Fatalf("count the monsters' NPCs: %v", err)
	}
	return n
}

// TestMR042_OneNpcPerCreatureIsReused: the NPC behind the monsters is one per campaign
// and creature, made on the first add and reused by every add and every combat; a
// second creature has its own. The copies have their own names and hit points.
func TestMR042_OneNpcPerCreatureIsReused(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.monsterSetup(t, false)
	a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) { r.Count = 2 })
	if n := a.monsterNpcs(t); n != 1 {
		t.Fatalf("NPCs for monsters after the first add = %d, want 1", n)
	}
	a.mustAddMonsters(t, a.get(t, a.master), func(r *playv1.AddMonstersRequest) { r.Count = 2 })
	if n := a.monsterNpcs(t); n != 1 {
		t.Errorf("NPCs for monsters after a second add of the same creature = %d, want 1", n)
	}
	// Another combat, the same creature: the same NPC, and the first combat's rows stay.
	a.endEncounter(t, a.get(t, a.master))
	e2 := a.monsterSetup(t, false)
	res := a.mustAddMonsters(t, e2, func(r *playv1.AddMonstersRequest) { r.Count = 1 })
	if n := a.monsterNpcs(t); n != 1 {
		t.Errorf("NPCs for monsters after a second combat = %d, want 1", n)
	}
	byLabel(t, res.GetEncounter(), "Bandido")
	// Two copies of the NPC with different hit points, in one combat.
	a.h.roller.queue(2, 2, 8, 8, 10, 10)
	two := a.mustAddMonsters(t, res.GetEncounter(), func(r *playv1.AddMonstersRequest) {
		r.Count = 2
		r.HitPoints = playv1.MonsterHitPoints_MONSTER_HIT_POINTS_ROLLED
	})
	if x, y := byLabel(t, two.GetEncounter(), "Bandido 2"), byLabel(t, two.GetEncounter(), "Bandido 3"); x.GetHitPointsMax() != 6 || y.GetHitPointsMax() != 18 || x.GetCharacterId() != y.GetCharacterId() {
		t.Errorf("the copies = %d and %d hit points, characters %s and %s; want 6 and 18 of one NPC", x.GetHitPointsMax(), y.GetHitPointsMax(), x.GetCharacterId(), y.GetCharacterId())
	}
	// A second creature: its own NPC.
	a.mustAddMonsters(t, a.get(t, a.master), func(r *playv1.AddMonstersRequest) { r.CreatureKey = "monster:wolf"; r.Count = 1 })
	if n := a.monsterNpcs(t); n != 2 {
		t.Errorf("NPCs for monsters with two creatures = %d, want 2", n)
	}
	if got := a.masterNpcs(t); got != 2 { // the Capitão and the Goblin of the fixture
		t.Errorf("the master lists %d NPCs, want his 2", got)
	}
}

// TestMR042_TwoAddsAtOnceKeepOneNpc: two adds of the same creature at the same time
// give four distinct labels and exactly one NPC.
func TestMR042_TwoAddsAtOnceKeepOneNpc(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 2)
	a := newArmed(t)
	e := a.monsterSetup(t, false)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	start := dbtest.NewBarrier(len(errs))
	for i := range errs {
		wg.Go(func() {
			start.Wait()
			_, errs[i] = a.addMonsters(t, e, newKey(), func(r *playv1.AddMonstersRequest) { r.Count = 2 })
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("AddMonsters(%d) error = %v", i, err)
		}
	}
	seen := map[string]bool{}
	for _, c := range a.get(t, a.master).GetCombatants() {
		if strings.HasPrefix(c.GetLabel(), "Bandido") {
			seen[c.GetLabel()] = true
		}
	}
	if len(seen) != 4 {
		t.Errorf("labels = %v, want 4 distinct Bandido labels", seen)
	}
	if n := a.monsterNpcs(t); n != 1 {
		t.Errorf("NPCs for monsters = %d, want 1", n)
	}
}

// TestMR042_AMonstersNpcIsNeverAParticipant: the NPC behind the monsters cannot be
// started with, added as a reinforcement or put on the stage, and AddMonsters is the
// only way in.
func TestMR042_AMonstersNpcIsNeverAParticipant(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.monsterSetup(t, true)
	res := a.mustAddMonsters(t, e, nil)
	npc := byLabel(t, res.GetEncounter(), "Bandido 1").GetCharacterId()
	if npc == "" {
		t.Fatal("the master's Bandido 1 has no character_id")
	}
	_, err := a.master.combat.AddCombatants(t.Context(), connect.NewRequest(&playv1.AddCombatantsRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), Participants: []*playv1.Participant{{CharacterId: npc}},
	}))
	wantCode(t, "AddCombatants(a monster's NPC)", err, connect.CodeInvalidArgument)
	a.endEncounter(t, e)
	_, err = a.master.combat.StartEncounter(t.Context(), connect.NewRequest(&playv1.StartEncounterRequest{
		CampaignId: a.campaignID, IdempotencyKey: newKey(), Name: "Outra", Participants: []*playv1.Participant{{CharacterId: npc}},
	}))
	wantCode(t, "StartEncounter(a monster's NPC)", err, connect.CodeInvalidArgument)
	point, _ := a.h.newScene(a.mapID, "A estrada", false, 1)
	a.openScene(t, point)
	_, err = a.master.play.PutOnStage(t.Context(), connect.NewRequest(&playv1.PutOnStageRequest{CampaignId: a.campaignID, CharacterId: npc}))
	wantCode(t, "PutOnStage(a monster's NPC)", err, connect.CodeInvalidArgument)
}

// TestMR042_ARetryIsCheckedAgainstWhatWasAdded: the same key with the same request
// answers the same; with another count, creature, name or hit points it is refused,
// and so is a key that an AddCombatants used.
func TestMR042_ARetryIsCheckedAgainstWhatWasAdded(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.monsterSetup(t, false)
	key := newKey()
	first, err := a.addMonsters(t, e, key, nil)
	if err != nil {
		t.Fatalf("AddMonsters() error = %v", err)
	}
	for name, edit := range map[string]func(*playv1.AddMonstersRequest){
		"another count":     func(r *playv1.AddMonstersRequest) { r.Count = 2 },
		"another creature":  func(r *playv1.AddMonstersRequest) { r.CreatureKey = "monster:wolf" },
		"another name":      func(r *playv1.AddMonstersRequest) { r.Name = "Salteador" },
		"rolled hit points": func(r *playv1.AddMonstersRequest) { r.HitPoints = playv1.MonsterHitPoints_MONSTER_HIT_POINTS_ROLLED },
		"revealed":          func(r *playv1.AddMonstersRequest) { r.Hidden = new(false) },
	} {
		if _, err := a.addMonsters(t, e, key, edit); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("a retry with %s = %v, want invalid_argument", name, err)
		}
	}
	// The default name written out is the same request.
	if again, err := a.addMonsters(t, e, key, func(r *playv1.AddMonstersRequest) {
		r.Name = "Bandido"
		r.HitPoints = playv1.MonsterHitPoints_MONSTER_HIT_POINTS_AVERAGE
	}); err != nil || strings.Join(again.GetCombatantIds(), ",") != strings.Join(first.GetCombatantIds(), ",") {
		t.Errorf("the same add written out = %v, %v; want the same ids", again.GetCombatantIds(), err)
	}
	// A key AddCombatants used.
	reinforcement := newKey()
	if _, err := a.master.combat.AddCombatants(t.Context(), connect.NewRequest(&playv1.AddCombatantsRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: reinforcement, Participants: []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
	})); err != nil {
		t.Fatalf("AddCombatants() error = %v", err)
	}
	if _, err := a.addMonsters(t, e, reinforcement, nil); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("AddMonsters on an AddCombatants key = %v, want invalid_argument", err)
	}
}

// hitDice finds the dice "2d8" written as dice, not inside a longer run of hex
// digits: every log entry carries random UUIDs, and one like "…e2d833…" must not
// fail the test (it did once, 07/10/2026).
var hitDice = regexp.MustCompile(`(^|[^0-9a-fA-F-])2d8`)

// TestMR042_OnlyTheMasterReadsTheCreatureAndItsHitDice: the master's combatant carries the
// creature's key and ND; a player's copy has neither, nor the monster's character,
// and the master's log line of the rolled hit points ("2d8 + 2: 3, 4") comes from
// before the combat begins and never reaches a player.
func TestMR042_OnlyTheMasterReadsTheCreatureAndItsHitDice(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.monsterSetup(t, true)
	a.h.roller.queue(3, 4, 8, 8, 10, 15)
	a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) {
		r.Count = 2
		r.HitPoints = playv1.MonsterHitPoints_MONSTER_HIT_POINTS_ROLLED
		r.Hidden = new(false)
	})
	e = a.begin(t, a.get(t, a.master))
	m := byLabel(t, a.get(t, a.master), "Bandido 1")
	if m.GetBestiaryCreatureKey() != bandit || m.GetChallengeRating() != "1/8" {
		t.Errorf("the master's Bandido 1 = creature %q, ND %q; want monster:bandit, 1/8", m.GetBestiaryCreatureKey(), m.GetChallengeRating())
	}
	seen := a.get(t, a.caio)
	raw, err := protojson.Marshal(seen)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, banned := range []string{"bestiaryCreatureKey", "challengeRating", "monster:bandit"} {
		if strings.Contains(string(raw), banned) {
			t.Errorf("a player's combat has %q: %s", banned, raw)
		}
	}
	if c := byLabel(t, seen, "Bandido 1"); c.GetCharacterId() != "" || c.GetBestiaryCreatureKey() != "" || c.GetChallengeRating() != "" {
		t.Errorf("a player's Bandido 1 = %v, want no character, creature or ND", c)
	}

	var line *playv1.CombatLogEntry
	for _, r := range a.log(t, a.master, e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_MONSTERS_ADDED {
				line = en
			}
		}
	}
	if line == nil || len(line.GetMonsters()) != 2 {
		t.Fatalf("the master's log has no MONSTERS_ADDED line with 2 monsters: %v", line)
	}
	for i, want := range []struct {
		label string
		hp    int32
		faces []int32
	}{{"Bandido 1", 9, []int32{3, 4}}, {"Bandido 2", 18, []int32{8, 8}}} {
		got := line.GetMonsters()[i]
		if got.GetLabel() != want.label || got.GetHitPoints() != want.hp || !got.GetRolled() || got.GetDice() != "2d8+2" || got.GetModifier() != 2 ||
			len(got.GetFaces()) != 2 || got.GetFaces()[0] != want.faces[0] || got.GetFaces()[1] != want.faces[1] {
			t.Errorf("line monster %d = %v, want %s with %d hit points, 2d8+2 rolled %v", i, got, want.label, want.hp, want.faces)
		}
	}
	for who, u := range map[string]*user{"Caio": a.caio, "Ana": a.ana} {
		logs, err := protojson.Marshal(a.log(t, u, e))
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		for _, banned := range []string{"MONSTERS_ADDED", "monsters", "faces", "hitPoints"} {
			if strings.Contains(string(logs), banned) {
				t.Errorf("%s's log has %q: %s", who, banned, logs)
			}
		}
		if hitDice.Match(logs) {
			t.Errorf("%s's log has the hit dice 2d8: %s", who, logs)
		}
	}
	// Average hit points leave a line too, with no dice.
	more := a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) { r.Count = 1; r.Name = "Capanga" })
	_ = more
	var avg *playv1.CombatLogEntry
	for _, r := range a.log(t, a.master, e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_MONSTERS_ADDED && en.GetMonsters()[0].GetLabel() == "Capanga" {
				avg = en
			}
		}
	}
	if avg == nil || avg.GetMonsters()[0].GetRolled() || avg.GetMonsters()[0].GetHitPoints() != 11 {
		t.Errorf("the average add's line = %v, want 11 hit points, not rolled", avg)
	}
}

// TestMR042_TheStreamCarriesNoMonster: while hidden monsters are added, and on a hidden
// monster's turn, a player's stream gets hints with no content: nothing of the
// monsters, their ids or the creature.
func TestMR042_TheStreamCarriesNoMonster(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.monsterSetup(t, true)
	w := a.caio.watch(t, a.campaignID)
	w.ready(t)
	a.h.roller.queue(20, 15, 4) // Bandido 1 plays first
	res := a.mustAddMonsters(t, e, nil)
	e = a.begin(t, a.get(t, a.master))
	if e.GetCurrentCombatantId() != res.GetCombatantIds()[0] {
		t.Fatalf("on turn = %s, want the hidden Bandido 1", e.GetCurrentCombatantId())
	}
	a.mustEndTurn(t, a.master, e) // the hidden monster's turn ends
	var all []*playv1.WatchGameSessionResponse
	deadline := time.After(10 * time.Second)
	for len(all) < 3 {
		select {
		case ev := <-w.events:
			if ev.GetHeartbeat() == nil {
				all = append(all, ev)
			}
		case <-deadline:
			t.Fatalf("only %d events: %v", len(all), kinds(all))
		}
	}
	text := asJSONAll(t, all)
	for _, id := range res.GetCombatantIds() {
		if strings.Contains(text, id) {
			t.Errorf("a player's stream has the hidden monster %s: %s", id, text)
		}
	}
	for _, banned := range []string{"Bandido", "bandit", "hitPoints", "armor"} {
		if strings.Contains(text, banned) {
			t.Errorf("a player's stream has %q: %s", banned, text)
		}
	}
	if !strings.Contains(strings.Join(kinds(all), ","), "encounter_changed") {
		t.Errorf("the player's stream = %v, want an encounter_changed hint", kinds(all))
	}
}

// banditScimitar is the Bandit's attack action: a monster of the combat attacks with its stat block's actions.
const banditScimitar = "monster:bandit#scimitar"

// TestMR042_MultiattackFollowsTheSRDCount: the turn options of a monster count as many
// attacks to the Attack action as the creature's Multiattack (the SRD's count: the
// Veterano 3, the Urso Pardo 2, the Montículo Movediço 2) and run out after the last.
// The master runs his monsters, so he is not refused a further attack (as with any
// NPC, "the master has the last word").
func TestMR042_MultiattackFollowsTheSRDCount(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	for _, tc := range []struct {
		key    string
		name   string
		n      int
		attack string
	}{{"monster:veteran", "Veterano", 3, "monster:veteran#longsword"}, {"monster:brown-bear", "Urso", 2, "monster:brown-bear#bite"}, {"monster:shambling-mound", "Monticulo", 2, "monster:shambling-mound#slam"}} {
		e := a.monsterSetup(t, true)
		a.h.roller.queue(20)
		a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) { r.CreatureKey, r.Count, r.Name = tc.key, 1, tc.name })
		e = a.begin(t, a.get(t, a.master))
		left := func() (perAction, atLeft int32) {
			eco := a.mustOptions(t, a.master, e, tc.name).GetOptions().GetEconomy()
			return eco.GetAttacksPerAction(), eco.GetAttacksLeft()
		}
		if per, atLeft := left(); int(per) != tc.n || int(atLeft) != tc.n {
			t.Fatalf("%s: %d attacks per action, %d left at the start; want %d", tc.name, per, atLeft, tc.n)
		}
		for i := 1; i <= tc.n; i++ {
			if _, err := a.attack(t, a.master, e, tc.name, tc.attack, "Toren", d20(1)); err != nil {
				t.Fatalf("%s: attack %d of %d error = %v", tc.name, i, tc.n, err)
			}
			if _, atLeft := left(); int(atLeft) != tc.n-i {
				t.Errorf("%s: %d attacks left after %d, want %d", tc.name, atLeft, i, tc.n-i)
			}
		}
		a.endEncounter(t, e)
	}
}

// A start with the most combatants the combat takes (4 players and 36 bandits) and rolled
// hit points: the master's lines of the monsters (each with its dice and faces) do not fit one
// event, so they are written as several.
func TestStartWithManyRolledMonstersKeepsEachEventWithinThePayloadLimit(t *testing.T) {
	t.Parallel()
	m := newMirathel(t)
	const n = 36 // 4 players + 36 = 40: the server's own TOO_MANY check accepts it (see TestMR043_TheFortyCountsThePartysCreatures)
	started, err := m.master.combat.StartEncounter(t.Context(), connect.NewRequest(&playv1.StartEncounterRequest{
		CampaignId: m.campaignID, IdempotencyKey: newKey(), Name: "x", Monsters: groups(bandit, n),
		MonsterHitPoints: playv1.MonsterHitPoints_MONSTER_HIT_POINTS_ROLLED,
	}))
	if err != nil {
		t.Fatalf("StartEncounter(%d rolled bandits, 4 players = 40 combatants) error = %v; want success (within the limit of 40)", n, err)
	}
	// The master's lines together list every monster, each with its dice.
	log, err := m.master.combat.ListCombatLog(t.Context(), connect.NewRequest(&playv1.ListCombatLogRequest{CampaignId: m.campaignID, EncounterId: started.Msg.GetEncounter().GetId()}))
	if err != nil {
		t.Fatalf("ListCombatLog() error = %v", err)
	}
	listed := 0
	for _, round := range log.Msg.GetRounds() {
		for _, entry := range round.GetEntries() {
			for _, mon := range entry.GetMonsters() {
				if mon.GetDice() == "" || len(mon.GetFaces()) == 0 {
					t.Errorf("the line of %s has no dice: %v", mon.GetLabel(), mon)
				}
				listed++
			}
		}
	}
	if listed != n {
		t.Errorf("the log lists %d monsters, want %d", listed, n)
	}
}

const poisonSpray = "spell:poison-spray"

// skeletonFight is a theatre combat with a Skeleton (13 hit points, vulnerable to
// bludgeoning, immune to poison) added by the master. Toren (a fighter with a mace)
// and Pensantus (a wizard with Poison Spray) are first in the order.
func skeletonFight(t *testing.T) (*armed, *playv1.Encounter, string) {
	t.Helper()
	scores := &rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 16, Wisdom: 10, Charisma: 8}
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, scores, []string{maceKey}, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1, scores, nil, []string{poisonSpray})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, scores, []string{rapier}, nil)
	})
	e := a.monsterSetup(t, true)
	a.h.roller.queue(1)
	res := a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) {
		r.CreatureKey = "monster:skeleton"
		r.Count = 1
		r.Hidden = new(false)
	})
	label := ""
	for _, c := range res.GetEncounter().GetCombatants() {
		if c.GetId() == res.GetCombatantIds()[0] {
			label = c.GetLabel()
		}
	}
	if label == "" {
		t.Fatal("the skeleton is not in the combat")
	}
	e = a.begin(t, a.get(t, a.master))
	if c := byLabel(t, e, label); c.GetHitPointsCurrent() != 13 {
		t.Fatalf("%s has %d hit points, want 13", label, c.GetHitPointsCurrent())
	}
	return a, e, label
}

// A monster takes double the damage of a type it is vulnerable to (SRD 5.1): the
// Skeleton and a mace hit.
func TestMonsterTakesDoubleDamageOfATypeItIsVulnerableTo(t *testing.T) {
	t.Parallel()
	a, e, skel := skeletonFight(t)
	key := a.mustOptions(t, a.caio, e, "Toren").GetOptions().GetAttacks()[0].GetAttack().GetKey()
	hit := a.mustAttack(t, a.caio, e, "Toren", key, skel, d20(15))
	pending := hit.GetPendingDamage().GetId()
	dmg := a.mustDamage(t, a.caio, e, pending, typedDamage(3)).GetPendingDamage()
	if dmg.GetDamageTypeKey() != "damage-type:bludgeoning" || dmg.GetAmount() < 1 {
		t.Fatalf("pending damage = %v, want bludgeoning", dmg)
	}
	if hp, _, _ := a.hp(t, skel); hp != 1 {
		t.Errorf("Skeleton has %d hit points after the hit, want 1 (13 minus 12)", hp)
	}
	if dmg.GetAmount() != 12 { // the mace's 3 on the die plus 3 from Strength, doubled
		t.Errorf("damage that lands = %d, want 12 (6 doubled)", dmg.GetAmount())
	}
}

// A monster immune to a damage type takes none of it (SRD 5.1): Poison Spray on a
// Skeleton.
func TestMonsterTakesNoDamageOfATypeItIsImmuneTo(t *testing.T) {
	t.Parallel()
	a, e, skel := skeletonFight(t)
	e = a.passTo(t, e, "Pensantus")
	// The skeleton fails its save (d20 = 1 queued), damage dice are 2d12.
	a.h.roller.queue(1, 6, 6)
	res, err := a.cast(t, a.ana, e, "Pensantus", poisonSpray, nil, a.at(t, skel), noCastRoll)
	if err != nil {
		t.Fatalf("CastSpell(Poison Spray) error = %v", err)
	}
	if len(res.GetCast().GetPendingDamages()) != 1 {
		t.Fatalf("cast = %v, want one pending poison damage", res.GetCast())
	}
	dmg := a.mustDamage(t, a.ana, e, res.GetCast().GetPendingDamages()[0].GetId(), typedDamage(6)).GetPendingDamage()
	if dmg.GetAmount() != 0 {
		t.Errorf("poison damage that lands = %d, want 0", dmg.GetAmount())
	}
	if hp, _, _ := a.hp(t, skel); hp != 13 {
		t.Errorf("Skeleton has %d hit points after Poison Spray, want 13", hp)
	}
}
