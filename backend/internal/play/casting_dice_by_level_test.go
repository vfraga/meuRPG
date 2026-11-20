package play

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The dice of a cast above the spell's level (#944): the options list them for each level the spell can be cast at, so
// a physical-dice table reads the count it types a sum for. These tests need the database (MEURPG_TEST_DATABASE_URL).

// castingSpell is the options' entry for one spell of a character.
func (a *armed) castingSpell(t *testing.T, u *user, c *charactersv1.Character, spell string) *playv1.CastingSpell {
	t.Helper()
	o, err := a.casting(t, u, c)
	if err != nil {
		t.Fatalf("GetCastOptions() error = %v", err)
	}
	for _, s := range o.GetSpells() {
		if s.GetSpell().GetKey() == spell {
			return s
		}
	}
	t.Fatalf("GetCastOptions() has no %s", spell)
	return nil
}

func diceAt(s *playv1.CastingSpell) map[int32]string {
	out := map[int32]string{}
	for _, d := range s.GetDiceByLevel() {
		out[d.GetLevel()] = d.GetDice()
	}
	return out
}

// Cure Wounds heals 1d8 for each level of the slot: the options list the dice of the 1st to the 9th level, and the
// roll_dice is the spell's own.
func TestCastingOptions_CureWoundsListsItsDiceForEachLevelFromItsOwn(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	s := a.castingSpell(t, a.bia, a.bri, cureWounds)
	if !s.GetRollsDice() || s.GetRollDice() != "1d8" {
		t.Errorf("cure wounds rolls_dice = %v, roll_dice = %q, want 1d8 at its own level", s.GetRollsDice(), s.GetRollDice())
	}
	got := diceAt(s)
	if len(s.GetDiceByLevel()) != 9 {
		t.Errorf("dice_by_level = %v, want one entry for each level from the 1st to the 9th", s.GetDiceByLevel())
	}
	for level := int32(1); level <= 9; level++ {
		if want := fmt.Sprintf("%dd8", level); got[level] != want {
			t.Errorf("dice at level %d = %q, want %q", level, got[level], want)
		}
	}
	if first := s.GetDiceByLevel(); len(first) > 0 && first[0].GetLevel() != 1 {
		t.Errorf("first entry = %v, want the spell's own level", first[0])
	}
}

// A spell that rolls no dice has no entries, a cantrip and a spell the combat narrates included.
func TestCastingOptions_ASpellThatRollsNoDiceHasNoDiceByLevel(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	for _, key := range []string{fireBolt, mageArmorKey} {
		if s := a.castingSpell(t, a.ana, a.pens, key); s.GetRollsDice() || len(s.GetDiceByLevel()) != 0 {
			t.Errorf("%s: rolls_dice = %v, dice_by_level = %v, want none", key, s.GetRollsDice(), s.GetDiceByLevel())
		}
	}
}

// The server's typed-sum bounds are the dice the options list for the cast level: below the count, above the
// highest face, and the extremes accepted.
func TestCastingOutside_ATypedSumFollowsTheDiceOfTheCastLevel(t *testing.T) {
	t.Parallel()
	a := newCastingParty(t)
	a.hurt(t, a.toren, 4)
	a.setPhysical(t, a.bia)
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	tor := []*charactersv1.Character{a.toren}
	typed := func(sum int32) func(*playv1.CastSpellOutsideCombatRequest) {
		return func(r *playv1.CastSpellOutsideCombatRequest) {
			r.Roll = &playv1.CastSpellOutsideCombatRequest_TypedSum{TypedSum: sum}
		}
	}
	// The 2nd-level slot: 2d8, from 2 to 16.
	if got := diceAt(a.castingSpell(t, a.bia, a.bri, cureWounds))[2]; got != "2d8" {
		t.Fatalf("dice at level 2 = %q, want 2d8", got)
	}
	_, err := a.castOut(t, a.bia, a.bri, cureWounds, slotOfLevel(2), false, tor, typed(1))
	wantCode(t, "a sum below the count of the cast level's dice", err, connect.CodeInvalidArgument)
	_, err = a.castOut(t, a.bia, a.bri, cureWounds, slotOfLevel(2), false, tor, typed(17))
	wantCode(t, "a sum above the cast level's dice", err, connect.CodeInvalidArgument)
	a.mustCastOut(t, a.bia, a.bri, cureWounds, slotOfLevel(2), false, tor, typed(16))
	if got := usedSlots(a.vitals(t, a.bri), 2); got != 1 {
		t.Errorf("2nd-level slots used = %d, want 1 (only the accepted cast spends)", got)
	}
}

// A grant that casts at a level lists that level's dice, and the server types the sum against them: 3d8 for a
// Cure Wounds granted at the 3rd level, so 2 is below the count and 24 is the highest.
func TestMR025_AGrantsCastLevelHasItsDiceInTheOptionsAndTheTypedSum(t *testing.T) {
	t.Parallel()
	a, feat := newLevelParty(t, cureWounds, 3)
	s := a.castingSpell(t, a.ana, a.pens, cureWounds)
	if len(s.GetGrants()) != 1 || s.GetGrants()[0].GetCastLevel() != 3 {
		t.Fatalf("grants = %v, want one at the cast level 3", s.GetGrants())
	}
	if got := diceAt(s); got[3] != "3d8" || got[1] != "1d8" || s.GetRollDice() != "1d8" {
		t.Errorf("dice at level 3 = %q, at level 1 = %q, roll_dice = %q, want 3d8, 1d8 and 1d8", got[3], got[1], s.GetRollDice())
	}
	a.setPhysical(t, a.ana)
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	typed := func(sum int32) func(*playv1.CastSpellOutsideCombatRequest) {
		return func(r *playv1.CastSpellOutsideCombatRequest) {
			r.GrantedBy = feat
			r.Roll = &playv1.CastSpellOutsideCombatRequest_TypedSum{TypedSum: sum}
		}
	}
	self := []*charactersv1.Character{a.pens}
	_, err := a.castOut(t, a.ana, a.pens, cureWounds, nil, false, self, typed(2))
	wantCode(t, "a sum below the grant's cast level dice", err, connect.CodeInvalidArgument)
	_, err = a.castOut(t, a.ana, a.pens, cureWounds, nil, false, self, typed(25))
	wantCode(t, "a sum above the grant's cast level dice", err, connect.CodeInvalidArgument)
	a.mustCastOut(t, a.ana, a.pens, cureWounds, nil, false, self, typed(24))
}
