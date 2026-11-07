package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The table die of applies "attack" a player adds to a weapon attack of its own character (RollAttackRequest.maneuver_key).
// Pensantus (Fighter 5, longsword, +5 to hit) has a d8 "Golpe Preciso" from a pool of two uses; the Goblin has armor class 12.

// strikeFight starts a combat where Pensantus is first and next to the Goblin.
func (g *rollDiceTable) strikeFight(t *testing.T) *playv1.Encounter {
	t.Helper()
	return g.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: g.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Pensantus": 20, "Toren": 9, "Brisa": 8},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Pensantus": {5, 5}, "Goblin": {6, 5}, "Toren": {3, 3}, "Brisa": {8, 8}},
	})
}

// strike attacks as Pensantus's player with a physical d20 and the faces of the extra dice, naming the maneuver.
func (g *rollDiceTable) strike(t *testing.T, e *playv1.Encounter, key string, d20Face int32, faces ...int32) (*playv1.RollAttackResponse, error) {
	t.Helper()
	return g.attack(t, g.ana, e, "Pensantus", longsword, "Goblin", func(r *playv1.RollAttackRequest) {
		d20(d20Face)(r)
		r.ManeuverKey, r.ExtraDieFaces = key, faces
	})
}

func TestADieOnAnAttackIsAddedToTheRollAndSpendsAUse(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.strikeFight(t)

	// A 6 and +5 is an 11 against armor class 12: the d8's 4 turns the miss into a hit.
	res, err := g.strike(t, e, g.strikeKey, 6, 4)
	if err != nil {
		t.Fatalf("RollAttack(with the die) error = %v", err)
	}
	roll := res.GetRoll()
	if roll.GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || res.GetPendingDamage() == nil {
		t.Fatalf("the attack = %v, want a hit", roll)
	}
	if want := 6 + roll.GetD20().GetModifier() + 4; roll.GetD20().GetTotal() != want {
		t.Errorf("total = %d, want the d20, the modifier and the die's 4 = %d", roll.GetD20().GetTotal(), want)
	}
	if got := roll.GetD20().GetExtraDice(); len(got) != 1 || got[0].GetFace() != 4 || got[0].GetFaces() != 8 || got[0].GetSign() != 1 || got[0].GetSourceNamePt() != "Golpe Preciso" {
		t.Errorf("extra dice = %v, want Golpe Preciso's d8 of 4", got)
	}
	if got := g.usesLeft(t); got != 1 {
		t.Errorf("uses left = %d, want 1", got)
	}
	// The second attack of the Attack action: a 2 and a 1 on the die (8 against 12) is still a miss, and the use is spent anyway.
	miss, err := g.strike(t, e, g.strikeKey, 2, 1)
	if err != nil {
		t.Fatalf("RollAttack(a die that does not reach) error = %v", err)
	}
	if miss.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS {
		t.Errorf("the attack = %v, want a miss", miss.GetRoll())
	}
	if got := g.usesLeft(t); got != 0 {
		t.Errorf("uses left = %d, want 0", got)
	}
}

func TestAnAttackWithoutTheDieIsAMissWhereTheDieHits(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.strikeFight(t)
	plain, err := g.attack(t, g.ana, e, "Pensantus", longsword, "Goblin", d20(6))
	if err != nil {
		t.Fatalf("RollAttack() error = %v", err)
	}
	if plain.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || len(plain.GetRoll().GetD20().GetExtraDice()) != 0 {
		t.Errorf("the attack without the die = %v, want a miss with no extra dice", plain.GetRoll())
	}
	if got := g.usesLeft(t); got != 2 {
		t.Errorf("uses left = %d, want 2 (an attack that names no die spends none)", got)
	}
}

func TestADieOnAnAttackIsRolledInTheApp(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.strikeFight(t)
	g.h.roller.queue(9, 6) // the d20, then the die
	res := g.mustAttack(t, g.ana, e, "Pensantus", longsword, "Goblin", func(r *playv1.RollAttackRequest) { inAppRoll(r); r.ManeuverKey = g.strikeKey })
	if got := res.GetRoll().GetD20().GetExtraDice(); len(got) != 1 || got[0].GetFace() != 6 {
		t.Errorf("extra dice = %v, want the app's d8 of 6", got)
	}
	if got := g.usesLeft(t); got != 1 {
		t.Errorf("uses left = %d, want 1", got)
	}
}

func TestADieOnAnAttackNeedsItsFaceWithPhysicalDice(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.strikeFight(t)
	for name, faces := range map[string][]int32{"no face": nil, "a face past the die": {9}} {
		if _, err := g.strike(t, e, g.strikeKey, 9, faces...); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: %v, want invalid_argument", name, err)
		}
	}
	if got := g.usesLeft(t); got != 2 {
		t.Errorf("uses left = %d, want 2 (a refused attack spends nothing)", got)
	}
}

func TestADieOnAnAttackIsRefusedWhenNoUseIsLeftOrItIsNotAnAttackDie(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.strikeFight(t)
	for name, key := range map[string]string{
		"a check die":       g.look,
		"a save die":        g.brace,
		"an initiative die": g.dash,
		"a key nobody has":  "feature:inventado@mesa",
	} {
		if _, err := g.strike(t, e, key, 9, 3); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: %v, want invalid_argument", name, err)
		}
	}
	if got := g.usesLeft(t); got != 2 {
		t.Fatalf("uses left = %d, want 2 (a refused attack spends nothing)", got)
	}
	for range 2 { // the two attacks of the action spend the pool
		if _, err := g.strike(t, e, g.strikeKey, 3, 1); err != nil {
			t.Fatalf("RollAttack(with the die) error = %v", err)
		}
	}
	g.passTo(t, e, "Goblin")
	g.passTo(t, e, "Pensantus") // the next round: the Attack action is whole again, the pool is not
	_, err := g.strike(t, e, g.strikeKey, 9, 3)
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_USES_LEFT)
}

func TestAnNPCAttackerAddsNoManeuverDie(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.castersFightNPCFirst(t) // the Capitão Goblin is on turn
	plain, err := g.attack(t, g.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))
	if err != nil || plain.GetRoll().GetD20() == nil {
		t.Fatalf("RollAttack(an NPC, no die) = %v, %v; want the roll", plain, err)
	}
	g.undoLast(t, e)
	_, err = g.attack(t, g.master, e, "Capitão Goblin", sword, "Pensantus", func(r *playv1.RollAttackRequest) {
		d20(9)(r)
		r.ManeuverKey, r.ExtraDieFaces = g.strikeKey, []int32{3}
	})
	if connect.CodeOf(err) != connect.CodeInvalidArgument || !strings.Contains(err.Error(), "goes on a weapon attack of a player's character") {
		t.Errorf("an NPC's attack with a maneuver die: %v, want invalid_argument naming the weapon attack of a player's character", err)
	}
}

func TestAnAttackThatIsUndoneGivesTheUseBack(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.strikeFight(t)
	if _, err := g.strike(t, e, g.strikeKey, 9, 4); err != nil {
		t.Fatalf("RollAttack() error = %v", err)
	}
	if got := g.usesLeft(t); got != 1 {
		t.Fatalf("uses left = %d, want 1", got)
	}
	id := g.log(t, g.master, g.get(t, g.master)).GetUndoableEventId()
	if id == "" {
		t.Fatal("nothing to undo after the attack")
	}
	if err := g.undo(t, g.master, e, id); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	if got := g.usesLeft(t); got != 2 {
		t.Errorf("uses left = %d after the undo, want 2", got)
	}
}

// A reaction window holds the roll: nothing is rolled or spent until the replay.
func TestAHeldAttackSpendsTheUseOnTheReplay(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	g.setRules(t, enemyReactionsAlways)
	e := g.strikeFight(t)
	held, err := g.strike(t, e, g.strikeKey, 9, 4)
	if err != nil {
		t.Fatalf("RollAttack() error = %v", err)
	}
	if !held.GetRoll().GetHeldForReaction() {
		t.Fatalf("the attack = %v, want it held for the master's check", held.GetRoll())
	}
	if got := g.usesLeft(t); got != 2 {
		t.Fatalf("uses left = %d while the roll is held, want 2", got)
	}
	w := g.windowOf(t, g.master, playv1.ReactionKind_REACTION_KIND_MASTER_CHECK)
	if w == nil {
		t.Fatal("the master has no check")
	}
	g.mustAnswer(t, g.master, e, w.GetId(), passAnswer)
	if got := g.usesLeft(t); got != 1 {
		t.Errorf("uses left = %d after the replay, want 1 (spent once)", got)
	}
	if got := extraDiceOfLog(g.logEntries(t, g.ana, e)); len(got) != 1 || got[0].GetFace() != 4 {
		t.Errorf("the log's dice = %v, want the d8 of 4 of the replayed attack", got)
	}
}

func TestAnOpportunityAttackTakesTheDie(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.strikeFight(t)
	g.passTo(t, e, "Goblin")
	offerID := g.mustOffer(t, e, "Goblin", "Pensantus").GetOpportunityOfferId()
	req := &playv1.RollAttackRequest{
		CampaignId: g.campaignID, EncounterId: e.GetId(), AttackerId: g.id(t, "Pensantus"), AttackKey: longsword, TargetId: g.id(t, "Goblin"),
		IdempotencyKey: newKey(), OpportunityOfferId: offerID, ManeuverKey: g.strikeKey, ExtraDieFaces: []int32{5},
	}
	d20(12)(req)
	res, err := g.ana.combat.RollAttack(t.Context(), connect.NewRequest(req))
	if err != nil {
		t.Fatalf("RollAttack(opportunity, with the die) error = %v", err)
	}
	if got := res.Msg.GetRoll().GetD20().GetExtraDice(); len(got) != 1 || got[0].GetFace() != 5 {
		t.Errorf("extra dice = %v, want the d8 of 5", got)
	}
	if got := g.usesLeft(t); got != 1 {
		t.Errorf("uses left = %d, want 1", got)
	}
}

func TestAnAttackAsAReactionTakesTheDie(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.strikeFight(t)
	g.mustEndTurn(t, g.ana, e)
	res, err := g.attackAs(t, g.ana, e, "Pensantus", longsword, "Goblin", func(r *playv1.RollAttackRequest) {
		d20(12)(r)
		r.ManeuverKey, r.ExtraDieFaces = g.strikeKey, []int32{2}
	}, true)
	if err != nil {
		t.Fatalf("RollAttack(as a reaction, with the die) error = %v", err)
	}
	if got := res.GetRoll().GetD20().GetExtraDice(); len(got) != 1 || got[0].GetFace() != 2 {
		t.Errorf("extra dice = %v, want the d8 of 2", got)
	}
	if got := g.usesLeft(t); got != 1 {
		t.Errorf("uses left = %d, want 1", got)
	}
}

// The die is a source of the master's and the attacker's player's log line, and of nobody else's.
func TestTheDieOfAnAttackIsInTheLogOfWhoSeesTheDice(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.strikeFight(t)
	if _, err := g.strike(t, e, g.strikeKey, 9, 4); err != nil {
		t.Fatalf("RollAttack() error = %v", err)
	}
	extra := func(u *user) []*playv1.ExtraDie { return extraDiceOfLog(g.logEntries(t, u, e)) }
	for name, u := range map[string]*user{"the master": g.master, "the attacker's player": g.ana} {
		if got := extra(u); len(got) != 1 || got[0].GetFace() != 4 || got[0].GetFaces() != 8 || got[0].GetSourceNamePt() != "Golpe Preciso" {
			t.Errorf("%s reads the log's dice %v, want the d8 of 4 named Golpe Preciso", name, got)
		}
	}
	if got := extra(g.caio); len(got) != 0 {
		t.Errorf("another player reads the log's dice %v, want none", got)
	}
	// A player never reads the armor class the die was compared with; the master does.
	res, err := g.strike(t, e, g.strikeKey, 3, 1)
	if err != nil {
		t.Fatalf("RollAttack() error = %v", err)
	}
	if res.GetRoll().TargetArmorClass != nil {
		t.Errorf("the attacker's player reads the armor class %d", res.GetRoll().GetTargetArmorClass())
	}
	for _, en := range g.logEntries(t, g.master, e) {
		if en.GetAttackRoll() != nil && en.TargetArmorClass != nil {
			return
		}
	}
	t.Error("the master's log has no armor class on the attack: the positive control is unreachable")
}

func TestAManeuverDieNeverGoesOnASpellAttackOrAnNPCsAttack(t *testing.T) {
	t.Parallel()
	player := playdb.Combatant{Kind: kindPlayer, CharacterID: "c1"}
	for _, tc := range []struct {
		name     string
		attacker playdb.Combatant
		attack   link.Attack
	}{
		{"a spell attack", player, link.Attack{Spell: true}},
		{"an NPC", playdb.Combatant{Kind: kindNPC, CharacterID: "c2"}, link.Attack{}},
	} {
		f := &attackFlow{req: &playv1.RollAttackRequest{ManeuverKey: "x"}, attacker: tc.attacker, attack: tc.attack}
		if err := f.checkManeuver(t.Context()); connect.CodeOf(err) != connect.CodeInvalidArgument || !strings.Contains(err.Error(), "goes on a weapon attack of a player's character") {
			t.Errorf("%s: %v, want invalid_argument naming the weapon attack of a player's character", tc.name, err)
		}
	}
	if err := (&attackFlow{req: &playv1.RollAttackRequest{}, attacker: playdb.Combatant{Kind: kindNPC}}).checkManeuver(t.Context()); err != nil {
		t.Errorf("an attack that names no die: %v, want none", err)
	}
}

// extraDiceOfLog are the extra dice of the latest attack the entries show.
func extraDiceOfLog(entries []*playv1.CombatLogEntry) []*playv1.ExtraDie {
	for _, en := range entries {
		if en.GetAttackRoll() != nil {
			return en.GetAttackRoll().GetExtraDice()
		}
	}
	return nil
}

// A natural 1 misses whatever the die adds, and the use is spent; a natural 20 is a critical hit whose dice are the
// weapon's alone: the maneuver die is not doubled.
func TestTheDieNeverTurnsAFumbleIntoAHitAndIsNotDoubledOnACriticalHit(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.strikeFight(t)
	fumble, err := g.strike(t, e, g.strikeKey, 1, 8) // 1 + 5 + 8 = 14 against armor class 12
	if err != nil {
		t.Fatalf("RollAttack(a 1 with the die) error = %v", err)
	}
	if fumble.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || fumble.GetPendingDamage() != nil {
		t.Errorf("the attack = %v, want a miss that opens no damage", fumble.GetRoll())
	}
	if got := g.usesLeft(t); got != 1 {
		t.Errorf("uses left = %d, want 1 (the die was rolled with the attack)", got)
	}
	crit, err := g.strike(t, e, g.strikeKey, 20, 8)
	if err != nil {
		t.Fatalf("RollAttack(a 20 with the die) error = %v", err)
	}
	if crit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_CRITICAL_HIT {
		t.Fatalf("the attack = %v, want a critical hit", crit.GetRoll())
	}
	if p := crit.GetPendingDamage(); p.GetDiceCount() != 2 || p.GetDiceSides() != 8 {
		t.Errorf("the damage = %d d%d, want the longsword's 1d8 doubled and nothing of the die", p.GetDiceCount(), p.GetDiceSides())
	}
}

// The dice an effect adds come first and the maneuver die last, in the typed faces too; the total counts both.
func TestTheDieComesAfterBlessInTheTypedFacesAndTheTotalCountsBoth(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.strikeFight(t)
	g.mustAddEffect(t, e, "spell:bless", []string{"Pensantus"}, g.rounds(t, 10, "Pensantus"))
	// The d4 of Bless takes the first face: a 6 there is past the die.
	if _, err := g.strike(t, e, g.strikeKey, 9, 6, 3); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("the faces in the wrong order: %v, want invalid_argument", err)
	}
	res, err := g.strike(t, e, g.strikeKey, 9, 3, 6)
	if err != nil {
		t.Fatalf("RollAttack(Bless and the die) error = %v", err)
	}
	roll := res.GetRoll().GetD20()
	dice := roll.GetExtraDice()
	if len(dice) != 2 || dice[0].GetFaces() != 4 || dice[0].GetFace() != 3 || dice[1].GetFaces() != 8 || dice[1].GetFace() != 6 {
		t.Fatalf("extra dice = %v, want Bless's d4 of 3 then the d8 of 6", dice)
	}
	if want := 9 + roll.GetModifier() + 3 + 6; roll.GetTotal() != want {
		t.Errorf("total = %d, want %d", roll.GetTotal(), want)
	}
	if got := g.usesLeft(t); got != 1 {
		t.Errorf("uses left = %d, want 1", got)
	}
}

// With disadvantage the d20 pair is typed first; the die is added to the d20 that counts.
func TestTheDieIsAddedToTheCountedD20OfADisadvantage(t *testing.T) {
	t.Parallel()
	g := newRollDiceTable(t)
	e := g.strikeFight(t)
	g.setConditions(t, e, "Pensantus", "condition:poisoned")
	res, err := g.attack(t, g.ana, e, "Pensantus", longsword, "Goblin", func(r *playv1.RollAttackRequest) {
		r.Roll, r.D20Faces = nil, []int32{15, 4}
		r.ManeuverKey, r.ExtraDieFaces = g.strikeKey, []int32{7}
	})
	if err != nil {
		t.Fatalf("RollAttack(with disadvantage and the die) error = %v", err)
	}
	roll := res.GetRoll()
	if roll.GetMode() != playv1.RollMode_ROLL_MODE_DISADVANTAGE {
		t.Fatalf("the mode = %v, want disadvantage", roll.GetMode())
	}
	if want := 4 + roll.GetD20().GetModifier() + 7; roll.GetD20().GetTotal() != want {
		t.Errorf("total = %d, want the 4 that counts, the modifier and the die's 7 = %d", roll.GetD20().GetTotal(), want)
	}
	if got := g.usesLeft(t); got != 1 {
		t.Errorf("uses left = %d, want 1", got)
	}
}

// strokeDieTable is a Rogue 20 (Stroke of Luck) of a subclass with an attack die, and two other heroes.
func newStrokeDieTable(t *testing.T) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		res, err := a.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
			CampaignId: a.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableSubclass{TableSubclass: &rulesv1.TableSubclass{
				NamePt: "Lâmina Certeira", ClassKey: "class:rogue", DescPt: []string{"Dados."},
				Levels: []*rulesv1.TableSubclassLevel{{Level: 3, Features: []*rulesv1.TableFeature{
					{NamePt: "Reserva", DescPt: []string{"Dados."}, Effects: []*rulesv1.TableEffect{{Type: "resource", Resource: rollDiceUses, Max: "2", Recharge: "short_rest"}}},
					{NamePt: "Golpe Preciso", DescPt: []string{"Um dado."}, Effects: []*rulesv1.TableEffect{{Type: "superiority_die", Applies: rules.ManeuverAttack, Resource: rollDiceUses, Value: "8"}}},
				}}},
			}},
		}))
		if err != nil {
			t.Fatalf("CreateTableEntry() error = %v", err)
		}
		sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 10}, RaceKey: "race:human",
			Background: &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
			Classes: []*charactersv1.ClassLevel{{
				ClassKey: "class:rogue", Level: 20, Subclass: &charactersv1.ClassLevel_SubclassKey{SubclassKey: res.Msg.GetEntry().GetKey()},
			}},
			Languages:            []string{"Anão", "Élfico", "Gnômico"},
			SkillProficiencyKeys: []string{"skill:acrobatics", "skill:perception", "skill:stealth", "skill:sleight-of-hand"},
			WeaponKeys:           []string{rapier},
			HitPoints:            &charactersv1.HitPoints{Method: charactersv1.HitPointsMethod_HIT_POINTS_METHOD_AVERAGE}, ExperiencePoints: 355000,
		}}}
		made, err := a.caio.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pipo", Sheet: sheet,
		}))
		if err != nil {
			t.Fatalf("CreateCharacter(Pipo) error = %v", err)
		}
		a.toren = made.Msg.GetCharacter()
		a.pens = a.ana.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 12, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 10}, []string{battleaxe}, nil)
		a.bri = a.bia.hero(t, a.campaignID, "Orla", "class:bard", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 8, Dexterity: 14, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 16}, nil, nil)
	})
}

func strokeDieKey(t *testing.T, a *armed) string {
	t.Helper()
	dice := a.toren.GetDerived().GetRollDice()
	if len(dice) != 1 {
		t.Fatalf("Pipo's dice = %v, want the attack die", dice)
	}
	return dice[0].GetKey()
}

// The die moves the total after the d20 is settled, so Stroke of Luck's question comes blind (even on a roll that hits),
// and the undo of an attack that used both gives back the use of the feature and the use of the die.
func TestAnAttackWithTheDieAsksStrokeOfLuckBlindAndTheUndoGivesBothBack(t *testing.T) {
	t.Parallel()
	a := newStrokeDieTable(t)
	key := strokeDieKey(t, a)
	e := a.strokeFight(t)
	withDie := func(face, die int32) (*playv1.RollAttackResponse, error) {
		return a.attack(t, a.caio, e, "Pipo", rapier, "Goblin", func(r *playv1.RollAttackRequest) {
			d20(face)(r)
			r.ManeuverKey, r.ExtraDieFaces = key, []int32{die}
		})
	}
	hit, err := withDie(19, 3)
	if err != nil {
		t.Fatalf("RollAttack(a 19 with the die) error = %v", err)
	}
	wantStroke(t, "the attack that hits with the die", hit.GetHeldRollOffer(), playv1.HeldRollKind_HELD_ROLL_KIND_WEAPON_ATTACK, false)
	if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_UNSPECIFIED {
		t.Fatalf("the attack was settled before the answer: %v", hit.GetRoll())
	}
	if _, err := a.answerHeldRoll(t, a.caio, hit.GetHeldRollOffer().GetHoldId(), strokeAnswer(false)); err != nil {
		t.Fatalf("AnswerHeldRoll(keep) error = %v", err)
	}
	if left, _ := poolOf(a.vitals(t, a.toren), rollDiceUses); left != 1 {
		t.Fatalf("dice left = %d after the kept roll, want 1", left)
	}
	a.passTo(t, e, "Toren")
	a.passTo(t, e, "Pipo") // the next round: the action is whole again
	// A natural 1 misses whatever the die adds: the question comes, the feature is used and the miss is a hit.
	miss, err := withDie(1, 1)
	if err != nil {
		t.Fatalf("RollAttack(a 1 with the die) error = %v", err)
	}
	wantStroke(t, "the attack that misses with the die", miss.GetHeldRollOffer(), playv1.HeldRollKind_HELD_ROLL_KIND_WEAPON_ATTACK, false)
	fin, err := a.answerHeldRoll(t, a.caio, miss.GetHeldRollOffer().GetHoldId(), strokeAnswer(true))
	if err != nil || fin.GetAttack().GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT {
		t.Fatalf("Stroke of Luck's answer = %v, %v; want the hit", fin, err)
	}
	if used := a.strokeUsed(t); used != 1 {
		t.Fatalf("stroke_of_luck used = %d, want 1", used)
	}
	if left, _ := poolOf(a.vitals(t, a.toren), rollDiceUses); left != 0 {
		t.Fatalf("dice left = %d, want 0", left)
	}
	a.undoLast(t, e)
	if used := a.strokeUsed(t); used != 0 {
		t.Errorf("stroke_of_luck used = %d after the undo, want 0", used)
	}
	if left, _ := poolOf(a.vitals(t, a.toren), rollDiceUses); left != 1 {
		t.Errorf("dice left = %d after the undo, want 1", left)
	}
}
