package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// plusHero creates a fighter that carries a rapier with the given magic bonus (full.item_bonuses).
func (u *user) plusHero(t *testing.T, campaignID, name string, bonus *charactersv1.ItemBonus) *charactersv1.Character {
	t.Helper()
	full := &charactersv1.FullSheet{
		BaseScores: abilities(16, 12, 14, 10, 8), RaceKey: "race:human",
		Classes:    []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: 2}},
		WeaponKeys: []string{rapier}, ItemBonuses: []*charactersv1.ItemBonus{bonus},
	}
	res, err := u.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: name,
		Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: full}},
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(%s) error = %v", name, err)
	}
	return res.Msg.GetCharacter()
}

// A +1 rapier reaches the combat through the same derived attack line as the sheet: its blow adds 1 to the damage
// and is magical, so a wight does not halve it (9 with the modifier, 10 with the +1, all of it); a +1 that asks for
// attunement the character lacks adds nothing, and the wight halves the plain 9.
func TestMagicBonusOfTheSheetReachesTheCombatDamage(t *testing.T) {
	t.Parallel()
	const wightHP = 45
	for name, tt := range map[string]struct {
		bonus *charactersv1.ItemBonus
		want  int32
	}{
		"plus one":            {&charactersv1.ItemBonus{Key: rapier, Bonus: 1}, wightHP - 10},
		"plus three":          {&charactersv1.ItemBonus{Key: rapier, Bonus: 3}, wightHP - 12},
		"needs attunement":    {&charactersv1.ItemBonus{Key: rapier, Bonus: 1, RequiresAttunement: true}, wightHP - 4},
		"needs it and has it": {&charactersv1.ItemBonus{Key: rapier, Bonus: 1, RequiresAttunement: true, Attuned: true}, wightHP - 10},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := hitFor(t, func(a *armed) { a.toren = a.caio.plusHero(t, a.campaignID, "Toren", tt.bonus) }, "monster:wight")
			if got != tt.want {
				t.Errorf("hit points after the blow = %d, want %d", got, tt.want)
			}
		})
	}
}

// The +N of a weapon is in its attack roll, not only in its damage: the same blow of a plain and of a +2 rapier differs by
// 2 in the d20's modifier, and the damage roll's modifier differs by 2 as well.
func TestMagicBonusOfTheSheetIsInTheAttackRoll(t *testing.T) {
	t.Parallel()
	modifiers := func(bonus *charactersv1.ItemBonus) (toHit, damage int32) {
		a, e, target := partsFight(t, func(a *armed) {
			if bonus == nil {
				a.toren = a.caio.markedHero(t, a.campaignID, "Toren", nil)
				return
			}
			a.toren = a.caio.plusHero(t, a.campaignID, "Toren", bonus)
		}, "monster:wight")
		hit := a.mustAttack(t, a.caio, e, "Toren", rapier, target, d20(15))
		dmg := a.mustDamage(t, a.caio, e, hit.GetPendingDamage().GetId(), inAppDamage).GetPendingDamage()
		return hit.GetRoll().GetD20().GetModifier(), dmg.GetRoll().GetModifier()
	}
	plainHit, plainDamage := modifiers(nil)
	plusHit, plusDamage := modifiers(&charactersv1.ItemBonus{Key: rapier, Bonus: 2})
	if plusHit != plainHit+2 || plusDamage != plainDamage+2 {
		t.Errorf("+2 rapier: to-hit %d (plain %d), damage modifier %d (plain %d), want both 2 higher", plusHit, plainHit, plusDamage, plainDamage)
	}
}

// The +N stays in the off-hand damage while the ability modifier goes: the bonus is part of the damage bonus, not of the
// ability modifier, which OffHandBonus takes away.
func TestMagicBonusStaysInTheOffHandDamage(t *testing.T) {
	t.Parallel()
	c, err := rules.LoadSRD()
	if err != nil {
		t.Fatalf("rules.LoadSRD() error = %v", err)
	}
	b := rules.Build{
		BaseScores: map[rules.Ability]int{rules.STR: 16, rules.DEX: 12, rules.CON: 14, rules.INT: 10, rules.WIS: 8, rules.CHA: 8},
		Race:       "race:human", Background: "background:acolyte",
		Classes:     []rules.ClassLevel{{Class: "class:fighter", Level: 2}},
		Weapons:     []string{"equipment:shortsword"},
		ItemBonuses: []rules.ItemBonus{{Key: "equipment:shortsword", Bonus: 2}},
	}
	for _, a := range rules.Derive(b, c).Attacks {
		if a.Key != "equipment:shortsword" {
			continue
		}
		if got, want := combat.OffHandBonus(a.DamageDice.Bonus, a.AbilityMod, false), 2; got != want {
			t.Errorf("off-hand damage bonus = %d, want %d (the +2 without the modifier %d)", got, want, a.AbilityMod)
		}
		return
	}
	t.Fatal("no shortsword line")
}
