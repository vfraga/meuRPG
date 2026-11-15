package play

import (
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

const wightHitPoints = 45

// The extra die of the weapon's type (Sneak Attack) takes the weapon's origin, part by part: against a wight
// (nonmagical weapons that aren't silvered) a plain rapier's whole 11 (3 and the Strength modifier 3 for the blade, the extra 5, all piercing) is halved to 5,
// a silvered or magical one's is not, and an adamantine mark does not help against the silver exemption.
func TestAnExtraDieOfTheWeaponsTypeTakesTheWeaponsOrigin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mark *charactersv1.WeaponMark
		want int32
	}{
		{"plain", nil, wightHitPoints - 5},
		{"silvered", &charactersv1.WeaponMark{WeaponKey: rapier, Silvered: true}, wightHitPoints - 11},
		{"adamantine only", &charactersv1.WeaponMark{WeaponKey: rapier, Adamantine: true}, wightHitPoints - 5},
		{"magical", &charactersv1.WeaponMark{WeaponKey: rapier, Magical: true}, wightHitPoints - 11},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a, e, wight := partsFight(t, func(a *armed) {
				a.toren = a.caio.markedHeroOf(t, a.campaignID, "Toren", "class:rogue", 3, tt.mark)
			}, "monster:wight")
			a.setConditions(t, e, wight, "condition:restrained") // advantage: Sneak Attack is allowed
			hit := a.mustAttack(t, a.caio, e, "Toren", rapier, wight, func(r *playv1.RollAttackRequest) { r.D20Faces = []int32{5, 18} })
			a.mustDamage(t, a.caio, e, hit.GetPendingDamage().GetId(), func(r *playv1.RollDamageRequest) {
				r.ExtrasChosen = true
				r.SelectedExtras = []*playv1.SelectedExtra{{Key: "sneak-attack"}}
				r.TypedParts = []*playv1.TypedPart{{PartKey: "weapon", Sum: 3}, {PartKey: "sneak-attack", Sum: 5}}
			})
			if hp, _, _ := a.hp(t, wight); hp != tt.want {
				t.Errorf("the wight has %d hit points, want %d", hp, tt.want)
			}
		})
	}
}

// Divine Smite's radiant damage is not the weapon's: an unsilvered rapier hit on a wight (it resists nonmagical
// piercing) halves only the weapon's own part, the smite and its extra die against the undead land whole.
func TestDivineSmiteRadiantIsNotReducedByTheNonmagicalResistanceToThePiercing(t *testing.T) {
	t.Parallel()
	a, e, wight := partsFight(t, func(a *armed) {
		a.toren = a.caio.markedHeroOf(t, a.campaignID, "Toren", "class:paladin", 3, nil)
	}, "monster:wight")
	hit := a.mustAttack(t, a.caio, e, "Toren", rapier, wight, d20(15))
	a.mustDamage(t, a.caio, e, hit.GetPendingDamage().GetId(), func(r *playv1.RollDamageRequest) {
		r.ExtrasChosen = true
		r.SelectedExtras = []*playv1.SelectedExtra{{Key: "divine-smite", SlotLevel: 1}}
		r.TypedParts = []*playv1.TypedPart{{PartKey: "weapon", Sum: 3}, {PartKey: "divine-smite", Sum: 9}, {PartKey: "divine-smite-extra", Sum: 4}}
	})
	// The weapon's 3 and its modifier (+3 Strength) are 6 piercing, halved to 3; the smite's 9 and 4 are radiant.
	if hp, _, _ := a.hp(t, wight); hp != wightHitPoints-(3+9+4) {
		t.Errorf("the wight has %d hit points, want %d (only the piercing halved)", hp, wightHitPoints-(3+9+4))
	}
}
