package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

const (
	arcanaKey    = "skill:arcana"
	acrobatics   = "skill:acrobatics"
	other        = playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_OTHER
	statueFindDC = 15
)

// searchWith is a search with another skill (OTHER and its key), in the app's roll
// or with the face of a real die.
func (r *trapRig) searchWith(t *testing.T, u *user, key string, face int32) (*playv1.SearchForTrapsResponse, error) {
	t.Helper()
	res, err := u.play.SearchForTraps(t.Context(), connect.NewRequest(&playv1.SearchForTrapsRequest{
		CampaignId: r.campaignID, IdempotencyKey: newKey(), Skill: other, OtherSkillKey: key, Roll: &playv1.SearchForTrapsRequest_D20Face{D20Face: face},
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// allowing is the edit that lets the given skills find a trap.
func allowing(keys ...string) func(*mapsv1.TrapSpec) {
	return func(s *mapsv1.TrapSpec) { s.AlsoFindSkillKeys = keys }
}

// TestAnotherSkillFindsOnlyTheTrapsThatAllowIt: Perception and Investigation find every
// trap; another skill finds only the traps whose master listed it, against the DC to
// find them. A skill no trap nearby allows answers exactly like a search that fell short
// (same shape, same cost): the player's menu is the same for every trap, so neither the
// menu nor the answer says whether a trap is there.
func TestAnotherSkillFindsOnlyTheTrapsThatAllowIt(t *testing.T) {
	t.Parallel()
	setup := func(t *testing.T, edit ...func(*mapsv1.TrapSpec)) (*trapRig, *mapsv1.MapPoint) {
		t.Helper()
		r := newTrapRig(t)
		statue := r.trap(t, "Estátua que cospe fogo", 9, 7, append([]func(*mapsv1.TrapSpec){func(s *mapsv1.TrapSpec) { s.NoticeDc, s.FindDc = statueFindDC, statueFindDC }}, edit...)...)
		r.place(t, r.pens.GetId(), 8, 7)
		return r, statue
	}
	t.Run("a listed skill finds it against the DC to find", func(t *testing.T) {
		t.Parallel()
		r, statue := setup(t, allowing(arcanaKey))
		low, err := r.searchWith(t, r.ana, arcanaKey, 1)
		if err != nil || len(low.GetFoundPointIds()) != 0 {
			t.Fatalf("a low Arcana roll = %v, %v; want nothing found", low, err)
		}
		high, err := r.searchWith(t, r.ana, arcanaKey, 20)
		if err != nil || len(high.GetFoundPointIds()) != 1 || high.GetFoundPointIds()[0] != statue.GetId() {
			t.Fatalf("a high Arcana roll = %v, %v; want the statue found", high, err)
		}
		if rev := r.revealedTo(t, statue.GetId()); len(rev) != 1 || rev[0].GetHow() != mapsv1.TrapRevealHow_TRAP_REVEAL_HOW_SEARCHED {
			t.Errorf("revealed to %v, want the searcher, searched", rev)
		}
	})
	t.Run("a skill nobody listed finds nothing, whatever the roll", func(t *testing.T) {
		t.Parallel()
		r, statue := setup(t, allowing(arcanaKey))
		miss, err := r.searchWith(t, r.ana, acrobatics, 20)
		if err != nil || len(miss.GetFoundPointIds()) != 0 || miss.GetSecondRoll() != nil || miss.GetRoll().GetFaces()[0] != 20 {
			t.Fatalf("an Acrobatics roll of 20 = %v, %v; want the roll answered and nothing found", miss, err)
		}
		if rev := r.revealedTo(t, statue.GetId()); len(rev) != 0 {
			t.Errorf("revealed to %v, want nobody", rev)
		}
	})
	t.Run("a trap with no list is found by Perception and Investigation only", func(t *testing.T) {
		t.Parallel()
		r, statue := setup(t)
		if res, err := r.searchWith(t, r.ana, arcanaKey, 20); err != nil || len(res.GetFoundPointIds()) != 0 {
			t.Fatalf("Arcana on a trap that lists nothing = %v, %v; want nothing found", res, err)
		}
		if res, err := r.search(t, r.ana, investigation, 20); err != nil || len(res.GetFoundPointIds()) != 1 || res.GetFoundPointIds()[0] != statue.GetId() {
			t.Fatalf("Investigation = %v, %v; want the statue found", res, err)
		}
	})
	t.Run("the answer is the shape of a search that fell short", func(t *testing.T) {
		t.Parallel()
		r, _ := setup(t, allowing(arcanaKey))
		denied, err := r.searchWith(t, r.ana, acrobatics, 20)
		if err != nil {
			t.Fatalf("SearchForTraps() error = %v", err)
		}
		short, err := r.search(t, r.ana, investigation, 1)
		if err != nil {
			t.Fatalf("SearchForTraps() error = %v", err)
		}
		if denied.GetSpentAction() != short.GetSpentAction() || (denied.GetSecondRoll() == nil) != (short.GetSecondRoll() == nil) ||
			len(denied.GetFoundPointIds()) != len(short.GetFoundPointIds()) || denied.GetRoll().GetDiceSides() != short.GetRoll().GetDiceSides() {
			t.Errorf("a refused skill answered %v, a short search %v; want the same shape", denied, short)
		}
	})
}

// TestAnotherSkillSearchRefusesWhatIsNotASkill: OTHER needs the key of an SRD skill that
// is neither Perception nor Investigation, and only OTHER takes a key.
func TestAnotherSkillSearchRefusesWhatIsNotASkill(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	r.place(t, r.pens.GetId(), 8, 7)
	for _, key := range []string{"", "skill:perception", "skill:investigation", "ability:int", "save:int", "skill:juggling", "arcana"} {
		_, err := r.searchWith(t, r.ana, key, 10)
		wantCode(t, "OTHER with "+key, err, connect.CodeInvalidArgument)
	}
	_, err := r.ana.play.SearchForTraps(t.Context(), connect.NewRequest(&playv1.SearchForTrapsRequest{
		CampaignId: r.campaignID, IdempotencyKey: newKey(), Skill: investigation, OtherSkillKey: arcanaKey, Roll: &playv1.SearchForTrapsRequest_D20Face{D20Face: 10},
	}))
	wantCode(t, "a key with INVESTIGATION", err, connect.CodeInvalidArgument)
	if n := r.eventCount(t, "trap_searched"); n != 0 {
		t.Errorf("trap_searched events = %d, want the refusals to leave none", n)
	}
}

// TestTheTrapsOtherSkillsAreTheMastersAlone: a trap's list is checked when the master
// saves it, comes back to the master, and no player ever receives it.
func TestTheTrapsOtherSkillsAreTheMastersAlone(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	statue := r.trap(t, "Estátua que cospe fogo", 9, 7, allowing(arcanaKey, "skill:religion"))
	if got := r.point(t, r.master, statue.GetId()).GetTrap().GetAlsoFindSkillKeys(); strings.Join(got, ",") != "skill:arcana,skill:religion" {
		t.Errorf("the master reads the list %v, want arcana and religion", got)
	}
	// A player who knows the trap gets the point, never the list.
	r.place(t, r.pens.GetId(), 8, 7)
	if res, err := r.searchWith(t, r.ana, arcanaKey, 20); err != nil || len(res.GetFoundPointIds()) != 1 {
		t.Fatalf("Arcana = %v, %v; want the statue found", res, err)
	}
	p := r.point(t, r.ana, statue.GetId())
	if p == nil || len(p.GetTrap().GetAlsoFindSkillKeys()) != 0 || p.GetTrap().GetFindDc() != 0 {
		t.Errorf("the finder reads the trap %v, want the point with no list and no DC", p.GetTrap())
	}
	// Refused when the master saves it.
	x, y := sq(12, 7)
	for name, keys := range map[string][]string{
		"perception":    {"skill:perception"},
		"investigation": {"skill:investigation"},
		"a repeat":      {arcanaKey, arcanaKey},
		"an ability":    {"ability:int"},
		"not a skill":   {"skill:juggling"},
		"too many": {
			"skill:acrobatics", "skill:animal-handling", arcanaKey, "skill:athletics", "skill:deception", "skill:history", "skill:insight", "skill:intimidation",
			"skill:medicine", "skill:nature", "skill:performance", "skill:persuasion", "skill:religion", "skill:sleight-of-hand", "skill:stealth", "skill:survival", "skill:perception",
		},
	} {
		spec := aTrap()
		spec.AlsoFindSkillKeys = keys
		_, err := r.mc(r.master).CreateMapPoint(t.Context(), connect.NewRequest(&mapsv1.CreateMapPointRequest{
			CampaignId: r.campaignID, MapId: r.mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, Name: "Armadilha", XBp: x, YBp: y, Trap: spec,
		}))
		wantCode(t, "a trap that lists "+name, err, connect.CodeInvalidArgument)
	}
	// An update replaces the list, and an empty one clears it.
	spec := r.point(t, r.master, statue.GetId()).GetTrap()
	spec.AlsoFindSkillKeys = nil
	if _, err := r.mc(r.master).UpdateMapPoint(t.Context(), connect.NewRequest(&mapsv1.UpdateMapPointRequest{
		CampaignId: r.campaignID, MapId: r.mapID, PointId: statue.GetId(), Trap: spec,
	})); err != nil {
		t.Fatalf("UpdateMapPoint() error = %v", err)
	}
	if got := r.point(t, r.master, statue.GetId()).GetTrap().GetAlsoFindSkillKeys(); len(got) != 0 {
		t.Errorf("the list after clearing it = %v, want none", got)
	}
}

// ---- Reliable Talent ----

// rogue is a human rogue of the given level proficient in four skills, with expertise in the
// first two (the first four from level 6, as the SRD 5.1 rogue gets).
func (u *user) rogue(t *testing.T, campaignID, name string, level int32) *charactersv1.Character {
	t.Helper()
	return u.rogueOf(t, campaignID, name, "race:human", level)
}

// rogueOf is rogue of another race (a halfling, for the Lucky tests).
func (u *user) rogueOf(t *testing.T, campaignID, name, race string, level int32) *charactersv1.Character {
	t.Helper()
	skills := []string{"skill:acrobatics", "skill:investigation", "skill:perception", "skill:stealth"}
	// SRD 5.1, Rogue, Expertise: two picks at level 1 and two more at level 6.
	expertise := 2
	if level >= 6 {
		expertise = 4
	}
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 14, Wisdom: 12, Charisma: 8},
		RaceKey:    race, Classes: []*charactersv1.ClassLevel{{ClassKey: "class:rogue", Level: level}},
		SkillProficiencyKeys: skills, ExpertiseSkillKeys: skills[:expertise],
	}}}
	res, err := u.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: name, Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(%s) error = %v", name, err)
	}
	return res.Msg.GetCharacter()
}

func wantTreated(t *testing.T, what string, roll *playv1.DiceRoll, face, counted int32) {
	t.Helper()
	if len(roll.GetFaces()) != 1 || roll.GetFaces()[0] != face {
		t.Errorf("%s: faces = %v, want [%d]", what, roll.GetFaces(), face)
	}
	if counted == 0 {
		if roll.TreatedAs != nil || roll.GetTreatedAsSource() != "" || roll.GetTotal() != face+roll.GetModifier() {
			t.Errorf("%s: roll = %v, want the d20 as it came up and no mention of the feature", what, roll)
		}
		return
	}
	if roll.TreatedAs == nil || roll.GetTreatedAs() != counted || roll.GetTreatedAsSource() != "feature:reliable-talent" || roll.GetTotal() != counted+roll.GetModifier() {
		t.Errorf("%s: roll = %v, want the %d counted as %d by Reliable Talent", what, roll, face, counted)
	}
}

// TestReliableTalentOnASceneCheck: a rogue of level 11 counts a d20 of 9 or lower as 10 on a
// skill check it is proficient in (SRD 5.1, Rogue: ability checks that add the
// proficiency bonus), with the app's dice and with a real die; the roll keeps what
// came up and says what it counted as only when the feature changed it. A skill
// without proficiency, a plain ability check, a saving throw and a rogue below level
// 11 get nothing.
func TestReliableTalentOnASceneCheck(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	dalila, verde := a.h.newUser("Dalila"), a.h.newUser("Verde")
	a.h.join(a.master, a.campaignID, dalila, verde)
	dalila.rogue(t, a.campaignID, "Dalila", 11)
	verde.rogue(t, a.campaignID, "Verde", 10)
	dc := new(int32(15))
	point, actions := a.h.newScene(a.mapID, "O muro do pátio", true, 0,
		sceneSpec{"skill:acrobatics", dc}, sceneSpec{"skill:arcana", dc}, sceneSpec{"ability:dex", dc}, sceneSpec{"save:dex", dc})
	a.openScene(t, point)
	roll := func(u *user, action string, edit func(*playv1.RollSceneCheckRequest)) *playv1.DiceRoll {
		t.Helper()
		req := &playv1.RollSceneCheckRequest{CampaignId: a.campaignID, ActionId: action, IdempotencyKey: newKey()}
		edit(req)
		res, err := u.play.RollSceneCheck(t.Context(), connect.NewRequest(req))
		if err != nil {
			t.Fatalf("RollSceneCheck() error = %v", err)
		}
		return res.Msg.GetRoll().GetRoll()
	}
	inApp := func(face int) func(*playv1.RollSceneCheckRequest) {
		return func(r *playv1.RollSceneCheckRequest) {
			a.h.roller.queue(face)
			r.Roll = &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true}
		}
	}
	typed := func(face int32) func(*playv1.RollSceneCheckRequest) {
		return func(r *playv1.RollSceneCheckRequest) { r.Roll = &playv1.RollSceneCheckRequest_D20Face{D20Face: face} }
	}

	wantTreated(t, "a 6 on Acrobatics", roll(dalila, actions[0], inApp(6)), 6, 10)
	wantTreated(t, "a 14 on Acrobatics", roll(dalila, actions[0], inApp(14)), 14, 0)
	wantTreated(t, "a 9 on Acrobatics", roll(dalila, actions[0], inApp(9)), 9, 10)
	wantTreated(t, "a 10 on Acrobatics", roll(dalila, actions[0], inApp(10)), 10, 0)
	wantTreated(t, "a typed 3 on Acrobatics", roll(dalila, actions[0], typed(3)), 3, 10)
	wantTreated(t, "a 6 on Arcana, no proficiency", roll(dalila, actions[1], inApp(6)), 6, 0)
	wantTreated(t, "a 6 on a plain Dexterity check", roll(dalila, actions[2], inApp(6)), 6, 0)
	wantTreated(t, "a 6 on a Dexterity saving throw", roll(dalila, actions[3], inApp(6)), 6, 0)
	wantTreated(t, "a 6 at rogue level 10", roll(verde, actions[0], inApp(6)), 6, 0)

	// The pass or fail follows the number that counted: 10 + the bonus against DC 15.
	res, err := dalila.play.RollSceneCheck(t.Context(), connect.NewRequest(&playv1.RollSceneCheckRequest{
		CampaignId: a.campaignID, ActionId: actions[0], IdempotencyKey: newKey(), Roll: &playv1.RollSceneCheckRequest_D20Face{D20Face: 2},
	}))
	if err != nil {
		t.Fatalf("RollSceneCheck() error = %v", err)
	}
	if want := res.Msg.GetRoll().GetRoll().GetTotal() >= 15; res.Msg.GetRoll().GetPassed() != want {
		t.Errorf("passed = %v for a total of %d against 15, want %v", res.Msg.GetRoll().GetPassed(), res.Msg.GetRoll().GetRoll().GetTotal(), want)
	}
}

// TestReliableTalentOnATrapSearch: the search counts a low d20 as 10 on a skill the rogue is
// proficient in, for each of the two dice of a Perception search with disadvantage
// before the lower is chosen; and the master's activity shows the same numbers.
func TestReliableTalentOnATrapSearch(t *testing.T) {
	t.Parallel()
	r := newTrapRig(t)
	hole := r.trap(t, "Fosso Dourado", 9, 7, func(s *mapsv1.TrapSpec) { s.NoticeDc, s.FindDc = 0, 14 }) // only a search finds it
	dalila := r.h.newUser("Dalila")
	r.h.join(r.master, r.campaignID, dalila)
	rogue := dalila.rogue(t, r.campaignID, "Dalila", 11)
	r.place(t, rogue.GetId(), 8, 7)

	res, err := r.search(t, dalila, investigation, 4)
	if err != nil {
		t.Fatalf("SearchForTraps() error = %v", err)
	}
	wantTreated(t, "a 4 on Investigation", res.GetRoll(), 4, 10)
	// 10 + the rogue's bonus (expertise: 4 + 2 × 4 at level 11) clears DC 14.
	if len(res.GetFoundPointIds()) != 1 || res.GetFoundPointIds()[0] != hole.GetId() {
		t.Errorf("found %v, want the pit: 10 counted for the 4", res.GetFoundPointIds())
	}
	var shown *playv1.TrapSearchResult
	for _, act := range r.activity(t, r.master) {
		if act.GetSearch() != nil {
			shown = act.GetSearch()
		}
	}
	if shown == nil {
		t.Fatal("the master's activity has no search")
	}
	wantTreated(t, "the master's line", shown.GetRoll(), 4, 10)
}
