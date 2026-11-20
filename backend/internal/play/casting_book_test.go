package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The warlock's Book of Ancient Secrets (SRD 5.1, Warlock, Eldritch Invocations): two 1st-level
// rituals of any class list that the warlock casts only as rituals, and, with the invocation, a
// warlock spell it knows can be cast as a ritual too. These tests need the database.

const (
	bookInvocation   = "feature:eldritch-invocation-book-of-ancient-secrets"
	bookPicks        = bookInvocation + "#rituals="
	comprehendKey    = "spell:comprehend-languages"
	charmPersonSpell = "spell:charm-person"
)

// warlockWithFeatures creates a level 3 warlock with the given feature choices, cantrips and spells.
func (u *user) warlockWithFeatures(t *testing.T, campaignID, name string, features, known []string) *charactersv1.Character {
	t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 16},
		RaceKey:    "race:human", Classes: []*charactersv1.ClassLevel{classLevel("class:warlock", 3, "subclass:fiend")},
		FeatureChoiceKeys: features, CantripKeys: []string{"spell:chill-touch", "spell:mage-hand"}, KnownSpellKeys: known,
	}}}
	res, err := u.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: name, Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(%s) error = %v", name, err)
	}
	return res.Msg.GetCharacter()
}

// TestCastingOutside_TheBookOfAncientSecretsCastsItsRitualsWithNoSlot: a spell of the book is cast
// as a ritual with no slot and ten more minutes, and with a slot it is refused as not the
// caster's; a warlock spell known with the ritual tag is cast as a ritual with the invocation and
// not without it.
func TestCastingOutside_TheBookOfAncientSecretsCastsItsRitualsWithNoSlot(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.warlockWithFeatures(t, a.campaignID, "Livro", []string{
			"feature:pact-of-the-tome", bookInvocation, "feature:eldritch-invocation-devils-sight",
			bookPicks + alarmKey, bookPicks + detectMagicKey,
		}, []string{comprehendKey, charmPersonSpell})
		a.bri = a.bia.warlockWithFeatures(t, a.campaignID, "Sem Livro", []string{
			"feature:pact-of-the-tome", "feature:eldritch-invocation-devils-sight", "feature:eldritch-invocation-beast-speech",
		}, []string{comprehendKey, charmPersonSpell})
	})
	pactUsed := func(c *charactersv1.Character) int32 { return a.vitals(t, c).GetPactSlots().GetUsed() }

	cast := a.mustCastOut(t, a.ana, a.pens, alarmKey, nil, true, nil).GetCast()
	if cast.GetStatus() != playv1.OutsideCastStatus_OUTSIDE_CAST_STATUS_CASTING || !cast.GetRitual() || cast.GetCastingMinutes() != 11 {
		t.Fatalf("cast = %v, want a ritual being cast, 11 minutes (1 + 10)", cast)
	}
	if got := pactUsed(a.pens); got != 0 {
		t.Errorf("pact slots used = %d, want 0: a ritual spends none", got)
	}
	if err := a.interrupt(t, a.ana, cast.GetId()); err != nil {
		t.Fatalf("InterruptCast() error = %v", err)
	}
	_, err := a.castOut(t, a.ana, a.pens, alarmKey, &playv1.SpellSlot{Level: 2, Pact: true}, false, nil)
	wantRefusal(t, "a slot cast of a spell of the book", err, connect.CodeInvalidArgument, "spell_key is not one of the caster's spells")
	_, err = a.castOut(t, a.ana, a.pens, alarmKey, nil, false, nil)
	wantRefusal(t, "a cast of a spell of the book that is no ritual", err, connect.CodeInvalidArgument, "spell_key is not one of the caster's spells")
	if got := pactUsed(a.pens); got != 0 {
		t.Errorf("pact slots used = %d after the refusals, want 0", got)
	}

	// A warlock spell that carries the tag: a ritual with the invocation, and only with the invocation.
	if r := a.mustCastOut(t, a.ana, a.pens, comprehendKey, nil, true, nil).GetCast(); r.GetCastingMinutes() != 10 || !r.GetRitual() {
		t.Errorf("Comprehend Languages as a ritual = %v, want 10 minutes (an action + 10)", r)
	}
	_, err = a.castOut(t, a.bia, a.bri, comprehendKey, nil, true, nil)
	wantCastingBlocked(t, "a known ritual of a warlock without the invocation", err, playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_CLASS_CANNOT_RITUAL)
	_, err = a.castOut(t, a.bia, a.bri, alarmKey, nil, true, nil)
	wantCastingBlocked(t, "a ritual of a warlock without the book", err, playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_CLASS_CANNOT_RITUAL)

	o, err := a.casting(t, a.ana, a.pens)
	if err != nil {
		t.Fatalf("GetCastOptions() error = %v", err)
	}
	by := map[string]*playv1.CastingSpell{}
	for _, s := range o.GetSpells() {
		by[s.GetSpell().GetKey()] = s
	}
	if s := by[detectMagicKey]; s == nil || s.GetCanCast() || !s.GetRitualAllowed() {
		t.Errorf("Detect Magic = %v, want a ritual only: in the book, not prepared", s)
	}
	if s := by[charmPersonSpell]; s == nil || !s.GetCanCast() || s.GetRitualAllowed() {
		t.Errorf("Charm Person = %v, want castable with a slot and no ritual", s)
	}
}

// bookParty is Toren the fighter and "Livro", a warlock with the book: the second ritual picks Alarm and
// Detect Magic. The warlock is complete at level 3, with 2,700 XP for level 4.
func bookParty(t *testing.T) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		tome := "feature:pact-of-the-tome#cantrips="
		sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 16},
			RaceKey:    "race:human", Classes: []*charactersv1.ClassLevel{classLevel("class:warlock", 3, "subclass:fiend")},
			FeatureChoiceKeys: []string{
				"feature:pact-of-the-tome", bookInvocation, "feature:eldritch-invocation-devils-sight", bookPicks + alarmKey, bookPicks + detectMagicKey,
				tome + "spell:guidance", tome + "spell:shillelagh", tome + "spell:light",
			},
			CantripKeys:      []string{"spell:chill-touch", "spell:mage-hand"},
			KnownSpellKeys:   []string{comprehendKey, charmPersonSpell, "spell:expeditious-retreat", "spell:hellish-rebuke"},
			ExperiencePoints: 2700,
		}}}
		res, err := a.ana.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Livro", Sheet: sheet,
		}))
		if err != nil {
			t.Fatalf("CreateCharacter(Livro) error = %v", err)
		}
		a.pens = res.Msg.GetCharacter()
	})
}

// TestCombat_ASpellOfTheBookIsNotCastInACombat: the book's spells are rituals and take ten more minutes, so
// the turn's spell list does not carry them and a cast of one with a slot is refused.
func TestCombat_ASpellOfTheBookIsNotCastInACombat(t *testing.T) {
	t.Parallel()
	a := bookParty(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Livro": 20, "Toren": 5},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Livro": {3, 3}, "Toren": {4, 3}, "Goblin": {5, 3}},
	})
	o := a.mustOptions(t, a.ana, e, "Livro")
	for _, k := range []string{alarmKey, detectMagicKey} {
		if s := spellOption(o, k); s != nil {
			t.Errorf("the turn's spells carry %s of the book: %v", k, s)
		}
	}
	if spellOption(o, charmPersonSpell) == nil {
		t.Error("the turn's spells lack Charm Person, which the warlock knows")
	}
	_, err := a.cast(t, a.ana, e, "Livro", alarmKey, &playv1.SpellSlot{Level: 2, Pact: true}, nil, noCastRoll)
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a combat cast of a spell of the book: error = %v, want invalid_argument", err)
	}
}

// TestCastingOutside_TheBookStillCastsItsRitualsAfterALevelUp: the picks stay on the sheet when the warlock
// takes a level, and the ritual still costs no slot.
func TestCastingOutside_TheBookStillCastsItsRitualsAfterALevelUp(t *testing.T) {
	t.Parallel()
	a := bookParty(t)
	made, err := a.ana.characters.LevelUpCharacter(t.Context(), connect.NewRequest(&charactersv1.LevelUpCharacterRequest{
		CampaignId: a.campaignID, CharacterId: a.pens.GetId(), Revision: a.pens.GetRevision(),
		Choices: &charactersv1.LevelUpChoices{
			ClassKey:        "class:warlock",
			AbilityIncrease: &rulesv1.AbilityScores{Charisma: 2},
			CantripKeys:     []string{"spell:minor-illusion"},
			KnownSpellKeys:  []string{"spell:illusory-script"},
			HitPoints:       &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
		},
	}))
	if err != nil {
		t.Fatalf("LevelUpCharacter() error = %v", err)
	}
	if got := made.Msg.GetCharacter().GetDerived().GetTotalLevel(); got != 4 {
		t.Fatalf("the level = %d, want 4", got)
	}
	a.pens = made.Msg.GetCharacter()
	cast := a.mustCastOut(t, a.ana, a.pens, alarmKey, nil, true, nil).GetCast()
	if !cast.GetRitual() || cast.GetCastingMinutes() != 11 {
		t.Errorf("the ritual after the level-up = %v, want 11 minutes", cast)
	}
	if got := a.vitals(t, a.pens).GetPactSlots().GetUsed(); got != 0 {
		t.Errorf("pact slots used = %d, want 0", got)
	}
}
