package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The effects that last, given by the master to a character outside a combat, and the ones a
// single roll spends (Guidance, Resistance: SRD 5.1 "the spell then ends"), the advantage
// Enhance Ability gives on the checks of one ability, and Heroism.

const (
	guidanceKey       = "spell:guidance"
	resistanceKey     = "spell:resistance"
	enhanceAbilityKey = "spell:enhance-ability"
	heroismKey        = "spell:heroism"
	longstriderKey    = "spell:longstrider"
)

// giveEffect has the master put an effect of the catalog on characters outside a combat.
func (a *armed) giveEffect(t *testing.T, key string, who []*charactersv1.Character, edit ...func(*playv1.AddCharacterEffectRequest)) (*playv1.AddCharacterEffectResponse, error) {
	t.Helper()
	req := &playv1.AddCharacterEffectRequest{CampaignId: a.campaignID, IdempotencyKey: newKey(), CatalogKey: key}
	for _, c := range who {
		req.CharacterIds = append(req.CharacterIds, c.GetId())
	}
	for _, e := range edit {
		e(req)
	}
	res, err := a.master.lasting.AddCharacterEffect(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (a *armed) mustGiveEffect(t *testing.T, key string, who []*charactersv1.Character, edit ...func(*playv1.AddCharacterEffectRequest)) string {
	t.Helper()
	res, err := a.giveEffect(t, key, who, edit...)
	if err != nil {
		t.Fatalf("AddCharacterEffect(%s) error = %v", key, err)
	}
	if len(res.GetEffectIds()) != len(who) {
		t.Fatalf("AddCharacterEffect(%s) made %v, want one for each character", key, res.GetEffectIds())
	}
	return res.GetEffectIds()[0]
}

func (a *armed) effectRows(t *testing.T, characterID, key string) int {
	t.Helper()
	var n int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM character_effects WHERE character_id = $1 AND source_key = $2`, characterID, key).Scan(&n); err != nil {
		t.Fatalf("count the effects: %v", err)
	}
	return n
}

// sceneRoll rolls the first action of the open scene as u with the app's dice (the queue holds
// the faces) and returns the roll.
func (a *armed) sceneRoll(t *testing.T, u *user, action string, edit ...func(*playv1.RollSceneCheckRequest)) (*playv1.SceneRoll, error) {
	t.Helper()
	req := &playv1.RollSceneCheckRequest{CampaignId: a.campaignID, ActionId: action, IdempotencyKey: newKey(), Roll: &playv1.RollSceneCheckRequest_RollInApp{RollInApp: true}}
	for _, e := range edit {
		e(req)
	}
	res, err := u.play.RollSceneCheck(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetRoll(), nil
}

func hasDieSource(roll *playv1.SceneRoll, want string) bool {
	for _, src := range roll.GetSources() {
		if src.GetKind() == playv1.AdvantageSourceKind_ADVANTAGE_SOURCE_KIND_EFFECT_DIE && strings.Contains(src.GetTextPt(), want) {
			return true
		}
	}
	return false
}

func TestGuidanceAddsAD4ToOneCheckAndThenEnds(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	a.mustGiveEffect(t, guidanceKey, []*charactersv1.Character{a.toren})
	point, actions := a.h.newScene(a.mapID, "Ponte", true, 3, sceneSpec{key: "skill:perception"}, sceneSpec{key: "save:wis"})
	a.openScene(t, point)
	// A saving throw does not take the die of Guidance, and keeps it.
	a.h.roller.queue(10)
	save, err := a.sceneRoll(t, a.caio, actions[1])
	if err != nil || hasDieSource(save, "1d4") {
		t.Fatalf("a save with Guidance = %v, %v; want no d4", save, err)
	}
	if a.effectRows(t, a.toren.GetId(), guidanceKey) != 1 {
		t.Fatal("a saving throw spent the Guidance")
	}
	a.h.roller.queue(3, 10) // the d4 is rolled first
	roll, err := a.sceneRoll(t, a.caio, actions[0])
	if err != nil || !hasDieSource(roll, "+1d4: 3") {
		t.Fatalf("a check with Guidance = %v, %v; want +1d4: 3", roll, err)
	}
	if got := a.effectRows(t, a.toren.GetId(), guidanceKey); got != 0 {
		t.Errorf("the Guidance stayed after the check: %d rows", got)
	}
	a.h.roller.queue(10)
	if roll, err = a.sceneRoll(t, a.caio, actions[0]); err != nil || hasDieSource(roll, "1d4") {
		t.Errorf("the second check = %v, %v; want no d4", roll, err)
	}
}

func TestResistanceAddsAD4ToOneSavingThrowAndThenEnds(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	a.mustGiveEffect(t, resistanceKey, []*charactersv1.Character{a.toren})
	point, actions := a.h.newScene(a.mapID, "Ponte", true, 3, sceneSpec{key: "save:wis"})
	a.openScene(t, point)
	a.h.roller.queue(2, 10)
	roll, err := a.sceneRoll(t, a.caio, actions[0])
	if err != nil || !hasDieSource(roll, "+1d4: 2") {
		t.Fatalf("a save with Resistance = %v, %v; want +1d4: 2", roll, err)
	}
	if got := a.effectRows(t, a.toren.GetId(), resistanceKey); got != 0 {
		t.Errorf("the Resistance stayed after the save: %d rows", got)
	}
}

func TestEnhanceAbilityGivesAdvantageOnTheChecksOfTheChosenAbility(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	if _, err := a.giveEffect(t, enhanceAbilityKey, []*charactersv1.Character{a.toren}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("Enhance Ability without an ability: error = %v, want invalid_argument", err)
	}
	a.mustGiveEffect(t, enhanceAbilityKey, []*charactersv1.Character{a.toren}, func(r *playv1.AddCharacterEffectRequest) { r.AbilityKey = "str" })
	point, actions := a.h.newScene(a.mapID, "Portão", true, 3, sceneSpec{key: "skill:athletics"}, sceneSpec{key: "skill:perception"})
	a.openScene(t, point)
	a.h.roller.queue(4, 15)
	roll, err := a.sceneRoll(t, a.caio, actions[0])
	if err != nil || roll.GetMode() != playv1.RollMode_ROLL_MODE_ADVANTAGE || len(roll.GetRoll().GetFaces()) != 2 {
		t.Fatalf("a Strength check = %v, %v; want advantage with two dice", roll, err)
	}
	a.h.roller.queue(4)
	roll, err = a.sceneRoll(t, a.caio, actions[1])
	if err != nil || roll.GetMode() == playv1.RollMode_ROLL_MODE_ADVANTAGE {
		t.Fatalf("a Wisdom check = %v, %v; want a normal roll", roll, err)
	}
}

func TestEnhanceAbilityOnAPuzzleHint(t *testing.T) {
	t.Parallel()
	p := newPuzzleTable(t)
	p.mustGiveEffect(t, enhanceAbilityKey, []*charactersv1.Character{p.toren}, func(r *playv1.AddCharacterEffectRequest) { r.AbilityKey = "int" })
	puz := p.lock(t, "Cofre", hintCheck("skill:arcana", 12))
	p.show(t, puz.GetId())
	p.h.roller.queue(3, 16)
	res, err := p.tryHint(t, p.caio, puz.GetId(), 0)
	if err != nil || len(res.GetRoll().GetFaces()) != 2 || !res.GetPassed() {
		t.Fatalf("a hint with Enhance Ability (Intelligence) = %v, %v; want two dice and the better one counting", res, err)
	}
}

func TestPhysicalDiceTypeTheEffectDieOutsideACombat(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	a.mustCastOut(t, a.bia, a.bri, blessKey, slotOfLevel(1), false, []*charactersv1.Character{a.toren})
	point, actions := a.h.newScene(a.mapID, "Ponte", true, 3, sceneSpec{key: "save:wis"})
	a.openScene(t, point)
	physical := func(face int32, extra ...int32) func(*playv1.RollSceneCheckRequest) {
		return func(r *playv1.RollSceneCheckRequest) {
			r.Roll = &playv1.RollSceneCheckRequest_D20Face{D20Face: face}
			r.ExtraDieFaces = extra
		}
	}
	if _, err := a.sceneRoll(t, a.caio, actions[0], physical(12)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("a physical save with the d4 missing: error = %v, want invalid_argument", err)
	}
	if _, err := a.sceneRoll(t, a.caio, actions[0], physical(12, 5)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("a d4 of 5: error = %v, want invalid_argument", err)
	}
	roll, err := a.sceneRoll(t, a.caio, actions[0], physical(12, 3))
	if err != nil || !hasDieSource(roll, "+1d4: 3") || !roll.GetRoll().GetPhysical() {
		t.Fatalf("a physical save with the d4 typed = %v, %v; want +1d4: 3", roll, err)
	}
}

func TestAShortRestEndsAnEffectOfOneMinute(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	a.mustCastOut(t, a.bia, a.bri, blessKey, slotOfLevel(1), false, []*charactersv1.Character{a.toren})
	if a.effectRows(t, a.toren.GetId(), blessKey) != 1 {
		t.Fatal("the Bless did not take hold")
	}
	if _, err := a.master.resource.TakeRest(t.Context(), connect.NewRequest(&playv1.TakeRestRequest{CampaignId: a.campaignID, Kind: playv1.RestKind_REST_KIND_SHORT, IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("TakeRest() error = %v", err)
	}
	if got := a.effectRows(t, a.toren.GetId(), blessKey); got != 0 {
		t.Errorf("a minute of Bless survived an hour: %d rows", got)
	}
}

func TestALongRestEndsTheEffectsThatLastUntilOneAndACombatKeepsThem(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	a.mustGiveEffect(t, "condition:poisoned", []*charactersv1.Character{a.toren}, func(r *playv1.AddCharacterEffectRequest) {
		r.DurationKind = playv1.EffectDurationKind_EFFECT_DURATION_KIND_LONG_REST
	})
	var kind string
	var seconds *int32
	read := func() {
		t.Helper()
		if err := a.h.pool.QueryRow(t.Context(), `SELECT duration_kind, seconds_left FROM character_effects WHERE character_id = $1`, a.toren.GetId()).Scan(&kind, &seconds); err != nil {
			t.Fatalf("read the effect: %v", err)
		}
	}
	read()
	if kind != "long_rest" || seconds != nil {
		t.Fatalf("the effect = %s, %v; want long_rest with no clock", kind, seconds)
	}
	// Time passes and nothing happens to it; a combat takes it in and gives it back whole.
	a.advanceGameTime(t, 3600)
	e := a.closeFight(t)
	var inCombat int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM combatant_states WHERE combatant_id = $1 AND source_key = 'condition:poisoned' AND duration_kind = 'long_rest'`, byLabel(t, e, "Toren").GetId()).Scan(&inCombat); err != nil || inCombat != 1 {
		t.Fatalf("Toren in the combat holds %d long-rest Poisoned (%v), want the same record", inCombat, err)
	}
	if _, err := a.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	read()
	if kind != "long_rest" || seconds != nil {
		t.Fatalf("after the combat the effect = %s, %v; want long_rest with no clock", kind, seconds)
	}
	if _, err := a.master.resource.TakeRest(t.Context(), connect.NewRequest(&playv1.TakeRestRequest{CampaignId: a.campaignID, Kind: playv1.RestKind_REST_KIND_SHORT, IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("TakeRest(short) error = %v", err)
	}
	if got := a.effectRows(t, a.toren.GetId(), "condition:poisoned"); got != 1 {
		t.Fatalf("a short rest left %d rows, want it to stay", got)
	}
	if _, err := a.master.resource.TakeRest(t.Context(), connect.NewRequest(&playv1.TakeRestRequest{CampaignId: a.campaignID, Kind: playv1.RestKind_REST_KIND_LONG, IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("TakeRest(long) error = %v", err)
	}
	if got := a.effectRows(t, a.toren.GetId(), "condition:poisoned"); got != 0 {
		t.Errorf("a long rest left %d rows, want none", got)
	}
}

func TestTheMasterGivesAnEffectToACharacterBetweenFights(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	hidden := func(r *playv1.AddCharacterEffectRequest) {
		r.PlayerVisible = new(false)
		r.Seconds, r.DurationKind = 600, playv1.EffectDurationKind_EFFECT_DURATION_KIND_ROUNDS
	}
	a.mustGiveEffect(t, blessKey, []*charactersv1.Character{a.toren}, hidden)
	var seconds int32
	var visible bool
	if err := a.h.pool.QueryRow(t.Context(), `SELECT seconds_left, player_visible FROM character_effects WHERE character_id = $1`, a.toren.GetId()).Scan(&seconds, &visible); err != nil || seconds != 600 || visible {
		t.Fatalf("the effect = %d seconds, visible %v (%v); want 600, hidden", seconds, visible, err)
	}
	list := func(u *user) int {
		res, err := u.lasting.ListCharacterEffects(t.Context(), connect.NewRequest(&playv1.ListCharacterEffectsRequest{CampaignId: a.campaignID}))
		if err != nil {
			t.Fatalf("ListCharacterEffects() error = %v", err)
		}
		return len(res.Msg.GetEffects())
	}
	if list(a.master) != 1 || list(a.caio) != 0 {
		t.Errorf("the master reads %d and the player %d; want 1 and 0 (RN-10)", list(a.master), list(a.caio))
	}
	// A player cannot give it, and a character in an open combat takes it there.
	if _, err := a.caio.lasting.AddCharacterEffect(t.Context(), connect.NewRequest(&playv1.AddCharacterEffectRequest{
		CampaignId: a.campaignID, IdempotencyKey: newKey(), CatalogKey: blessKey, CharacterIds: []string{a.toren.GetId()},
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("a player giving an effect: error = %v, want permission_denied", err)
	}
	a.closeFight(t)
	if _, err := a.giveEffect(t, blessKey, []*charactersv1.Character{a.toren}); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("giving an effect to a character in a combat: error = %v, want failed_precondition", err)
	}
}

func TestLongstriderAddsTenFeetOfSpeedInTheCombat(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	a.mustGiveEffect(t, longstriderKey, []*charactersv1.Character{a.toren})
	e := a.closeFight(t)
	var add int32
	if err := a.h.pool.QueryRow(t.Context(), `SELECT effect_speed_add_ft FROM combatants WHERE id = $1`, byLabel(t, e, "Toren").GetId()).Scan(&add); err != nil || add != 10 {
		t.Errorf("Toren's speed bonus in the combat = %d (%v), want 10 feet", add, err)
	}
	// Speed is in tenths of a foot: 30 ft + 10 ft for Toren, 30 ft for Pensantus.
	if got, other := byLabel(t, e, "Toren").GetSpeedDft(), byLabel(t, e, "Pensantus").GetSpeedDft(); got != other+100 {
		t.Errorf("Toren's speed = %d, want %d (Pensantus's plus 10 ft)", got, other+100)
	}
}

func TestHeroismMakesImmuneToFrightenedAndGivesTempHPAtEachTurn(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	e := a.closeFight(t)
	a.mustAddEffect(t, e, heroismKey, []string{"Toren"}, func(r *playv1.AddLastingEffectRequest) { r.CasterId = a.id(t, "Pensantus") })
	// Frightened set by hand on the hero does not stay.
	a.setConditions(t, e, "Toren", "condition:frightened")
	if hasCondition(a.get(t, a.master), t, "Toren", "condition:frightened") {
		t.Error("a Heroism target got Frightened")
	}
	// At the start of its next turn it gains the caster's spellcasting modifier (Intelligence 16: +3).
	e = a.turnOf(t, "Toren")
	if got := a.vitals(t, a.toren).GetHitPointsTemporary(); got != 3 {
		t.Errorf("Toren's temporary hit points at the start of the turn = %d, want 3", got)
	}
	// The spell ends: what it gave goes.
	f := cardOf(t, byLabel(t, e, "Toren"), heroismKey)
	if f == nil {
		t.Fatalf("Toren has no Heroism: %v", byLabel(t, e, "Toren").GetEffects())
	}
	if _, err := a.master.lasting.EndLastingEffect(t.Context(), connect.NewRequest(&playv1.EndLastingEffectRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), EffectId: f.GetId(), Scope: playv1.EffectEndScope_EFFECT_END_SCOPE_THIS, IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndLastingEffect() error = %v", err)
	}
	if got := a.vitals(t, a.toren).GetHitPointsTemporary(); got != 0 {
		t.Errorf("Toren's temporary hit points after the spell = %d, want 0", got)
	}
}

func TestADefeatedPlayersCheckReadsItsCombatEffects(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	e := a.closeFight(t)
	a.mustAddEffect(t, e, blessKey, []string{"Toren"}, a.rounds(t, 10, "Toren"))
	a.execSQL(t, `UPDATE combatants SET defeated = true WHERE id = $1`, byLabel(t, e, "Toren").GetId())
	point, actions := a.h.newScene(a.mapID, "Ponte", true, 3, sceneSpec{key: "save:wis"})
	a.openScene(t, point)
	a.h.roller.queue(4, 10)
	roll, err := a.sceneRoll(t, a.caio, actions[0])
	if err != nil || !hasDieSource(roll, "+1d4: 4") {
		t.Fatalf("a save by a defeated hero with Bless = %v, %v; want +1d4: 4", roll, err)
	}
}

// newBlessersParty is a cleric who knows Guidance and prepares Enhance Ability.
func newBlessersParty(t *testing.T) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.bri = a.bia.castingHero(t, a.campaignID, "Brisa", classLevel("class:cleric", 3, "subclass:life"),
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{sacredFlame, guidanceKey, resistanceKey}, nil,
			[]string{cureWounds, enhanceAbilityKey}, "")
	})
}

func TestCastingGuidanceAndEnhanceAbilityOutsideACombatPutsTheEffectOnTheCharacter(t *testing.T) {
	t.Parallel()
	a := newBlessersParty(t)
	cast := a.mustCastOut(t, a.bia, a.bri, guidanceKey, nil, false, []*charactersv1.Character{a.toren}).GetCast()
	if cast.GetDurationSeconds() != 60 {
		t.Errorf("Guidance lasts %d seconds, want 60", cast.GetDurationSeconds())
	}
	if a.effectRows(t, a.toren.GetId(), guidanceKey) != 1 {
		t.Fatal("Guidance put no effect on Toren")
	}
	// Enhance Ability needs the ability.
	if _, err := a.castOut(t, a.bia, a.bri, enhanceAbilityKey, slotOfLevel(2), false, []*charactersv1.Character{a.toren}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("Enhance Ability with no ability: error = %v, want invalid_argument", err)
	}
	a.mustCastOut(t, a.bia, a.bri, enhanceAbilityKey, slotOfLevel(2), false, []*charactersv1.Character{a.toren}, func(r *playv1.CastSpellOutsideCombatRequest) { r.AbilityKey = "dex" })
	var mods string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT modifiers::TEXT FROM character_effects WHERE character_id = $1 AND source_key = $2`, a.toren.GetId(), enhanceAbilityKey).Scan(&mods); err != nil || !strings.Contains(mods, `"dex"`) {
		t.Errorf("the Enhance Ability modifiers = %s (%v), want Dexterity", mods, err)
	}
}

func TestTheCatalogComesWithoutACombatAndHoldsTheNewSpells(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	res, err := a.master.lasting.ListLastingEffects(t.Context(), connect.NewRequest(&playv1.ListLastingEffectsRequest{CampaignId: a.campaignID}))
	if err != nil {
		t.Fatalf("ListLastingEffects() with no combat error = %v", err)
	}
	have := map[string]bool{}
	for _, e := range res.Msg.GetCatalog() {
		have[e.GetKey()] = true
	}
	for _, k := range []string{guidanceKey, resistanceKey, enhanceAbilityKey, heroismKey, longstriderKey, blessKey, shieldOfFaithKey} {
		if !have[k] {
			t.Errorf("the catalog lacks %s", k)
		}
	}
	if len(res.Msg.GetEffects()) != 0 {
		t.Errorf("effects with no combat = %v, want none", res.Msg.GetEffects())
	}
}

// TestRN10_AnotherPlayersEffectsOutsideACombat: a player reads the effects "for everyone" on the other
// characters, each with the character it is on (the sheet's "Seus efeitos" keeps only its own), and never reads an effect the
// master left for the owner alone.
func TestRN10_AnotherPlayersEffectsOutsideACombat(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	a.mustGiveEffect(t, blessKey, []*charactersv1.Character{a.pens}, func(r *playv1.AddCharacterEffectRequest) {
		r.Audience = playv1.EffectAudience_EFFECT_AUDIENCE_ALL
	})
	a.mustGiveEffect(t, "condition:poisoned", []*charactersv1.Character{a.pens}, func(r *playv1.AddCharacterEffectRequest) {
		r.Audience = playv1.EffectAudience_EFFECT_AUDIENCE_OWNER
	})
	read := func(u *user) map[string]*playv1.CharacterEffect {
		t.Helper()
		res, err := u.lasting.ListCharacterEffects(t.Context(), connect.NewRequest(&playv1.ListCharacterEffectsRequest{CampaignId: a.campaignID}))
		if err != nil {
			t.Fatalf("ListCharacterEffects() error = %v", err)
		}
		out := map[string]*playv1.CharacterEffect{}
		for _, e := range res.Msg.GetEffects() {
			out[e.GetSourceKey()] = e
		}
		return out
	}
	other := read(a.caio)
	if e := other[blessKey]; e == nil || e.GetCharacterId() != a.pens.GetId() {
		t.Errorf("Caio reads Bless on Pensantus as %v; want it there, on Pensantus", e)
	}
	if other["condition:poisoned"] != nil {
		t.Error("Caio read an effect left for Pensantus's owner alone (RN-10)")
	}
	own := read(a.ana)
	if own[blessKey] == nil || own["condition:poisoned"] == nil {
		t.Errorf("Ana reads %v; want both effects on her character", own)
	}
}

// TestAConcentrationEffectWithNoCasterInTheCombatMakesNoOneConcentrate: the master adds Bless for a caster who is not in
// the fight; the target keeps the effect and does not concentrate on it (SRD 5.1: the caster concentrates).
func TestAConcentrationEffectWithNoCasterInTheCombatMakesNoOneConcentrate(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	e := a.closeFight(t)
	a.mustAddEffect(t, e, blessKey, []string{"Toren"}, a.rounds(t, 10, "Toren"))
	e = a.get(t, a.master)
	toren := byLabel(t, e, "Toren")
	if cardOf(t, toren, blessKey) == nil {
		t.Fatalf("Toren has no Bless: %v", toren.GetEffects())
	}
	for _, c := range e.GetCombatants() {
		if c.GetConcentrationSpell() != "" {
			t.Errorf("%s concentrates on %q, but the caster is outside the combat", c.GetLabel(), c.GetConcentrationSpell())
		}
	}
	if got := byLabel(t, a.get(t, a.caio), "Toren").GetConcentrationSpell(); got != "" {
		t.Errorf("the player reads Toren concentrating on %q, want none", got)
	}
}
