package play

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A long casting through a granted_spell effect finishes at the cast level of the grant it started through (MR-025),
// also for a spell the character has prepared. These tests need the database (MEURPG_TEST_DATABASE_URL).

// newClericGrants is newCasters where Brisa (a cleric with Prayer of Healing prepared) has one feat for each of the
// cast levels: each feat grants Prayer of Healing at that level, twice a long rest (the resource prayerUses(level)),
// through Wisdom. It returns the feat keys in order.
func newClericGrants(t *testing.T, castLevels ...int32) (a *armed, feats []string) {
	t.Helper()
	a = newCastingParty(t)
	for _, lvl := range castLevels {
		res, err := a.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
			CampaignId: a.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableFeat{TableFeat: &rulesv1.TableFeat{
				NamePt: fmt.Sprintf("Prece de teste %d", lvl), DescPt: []string{"Um dom de teste."},
				Effects: []*rulesv1.TableEffect{
					{Type: "resource", Resource: prayerUses(lvl), Max: "2", Recharge: "long_rest"},
					{Type: "granted_spell", Spell: prayerKey, Ability: "wis", Uses: prayerUses(lvl), CastLevel: lvl},
				},
			}},
		}))
		if err != nil {
			t.Fatalf("CreateTableEntry(feat) error = %v", err)
		}
		feats = append(feats, res.Msg.GetEntry().GetKey())
	}
	a.bri = a.briWithFeats(t, feats, true)
	return a, feats
}

func prayerUses(lvl int32) string { return fmt.Sprintf("usos_prece_%d", lvl) }

// castState is Toren's hit points, Brisa's slots of levels 1 and 2 and the uses of the given grants she has spent.
func (a *armed) castState(t *testing.T, levels []int32) string {
	t.Helper()
	b := a.vitals(t, a.bri)
	s := fmt.Sprintf("Toren %d hp, slots used %d/%d", a.vitals(t, a.toren).GetHitPointsCurrent(), usedSlots(b, 1), usedSlots(b, 2))
	for _, lvl := range levels {
		used, _ := resourceUsed(b, prayerUses(lvl))
		s += fmt.Sprintf(", %s used %d", prayerUses(lvl), used)
	}
	return s
}

// briWithFeats is Brisa after the master sets the feats of her sheet.
func (a *armed) briWithFeats(t *testing.T, feats []string, prayerPrepared bool) *charactersv1.Character {
	t.Helper()
	prepared := []string{cureWounds, healingWord, detectMagicKey, blessKey, aidSpell}
	if prayerPrepared {
		prepared = append(prepared, prayerKey)
	}
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8},
		RaceKey:    "race:human", Classes: []*charactersv1.ClassLevel{classLevel("class:cleric", 3, "subclass:life")},
		CantripKeys: []string{sacredFlame}, PreparedSpellKeys: prepared,
		FeatKeys: feats,
	}}}
	upd, err := a.master.characters.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: a.campaignID, CharacterId: a.bri.GetId(), Revision: a.bri.GetRevision(), Name: "Brisa", Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("UpdateCharacter(Brisa gets the feats) error = %v", err)
	}
	return upd.Msg.GetCharacter()
}

// prayerAt starts a long casting of Prayer of Healing through the grant, finishes it as the master and says how many
// dice it healed with, and that it spent no slot.
func (a *armed) prayerAt(t *testing.T, feat string) int32 {
	t.Helper()
	start := a.mustCastOut(t, a.bia, a.bri, prayerKey, nil, false, []*charactersv1.Character{a.toren}, grantedBy(feat)).GetCast()
	if start.GetStatus() != playv1.OutsideCastStatus_OUTSIDE_CAST_STATUS_CASTING {
		t.Fatalf("cast = %v, want a casting that goes on", start)
	}
	a.hurt(t, a.toren, 20)
	done, err := a.finish(t, a.master, start.GetId())
	if err != nil {
		t.Fatalf("ConfirmCastTimePassed() error = %v", err)
	}
	for lvl := int32(1); lvl <= 2; lvl++ {
		if usedSlots(a.vitals(t, a.bri), lvl) != 0 {
			t.Errorf("a slot of level %d was spent by a cast through a grant", lvl)
		}
	}
	return done.GetCast().GetDiceCount()
}

// A cleric who has Prayer of Healing prepared and a grant of it at the 4th level: the casting started through the
// grant finishes at the 4th level, not at the spell's own.
func TestMR025_ALongCastThroughAGrantOfAPreparedSpellFinishesAtTheGrantsLevel(t *testing.T) {
	t.Parallel()
	a, feats := newClericGrants(t, 4)
	if got := a.prayerAt(t, feats[0]); got != 4 {
		t.Errorf("dice of the finished casting = %d, want 4 (2d8 and one more for each level above the 2nd, at the 4th)", got)
	}
}

// Two grants of one spell from two table entries, at different cast levels: each long casting finishes at the level of
// the grant it started through.
func TestMR025_TwoGrantsOfOneSpellFinishEachAtItsOwnLevel(t *testing.T) {
	t.Parallel()
	a, feats := newClericGrants(t, 3, 5)
	if got := a.prayerAt(t, feats[1]); got != 5 {
		t.Errorf("dice of the casting through the second grant = %d, want 5", got)
	}
	if got := a.prayerAt(t, feats[0]); got != 3 {
		t.Errorf("dice of the casting through the first grant = %d, want 3", got)
	}
}

// startPrayer starts a long casting of Prayer of Healing through the grant and returns the cast's id.
func (a *armed) startPrayer(t *testing.T, feat string) string {
	t.Helper()
	return a.mustCastOut(t, a.bia, a.bri, prayerKey, nil, false, []*charactersv1.Character{a.toren}, grantedBy(feat)).GetCast().GetId()
}

// A casting whose grant is gone before the master finishes it is refused, and the cast goes on: with another grant of the
// spell left (prepared or not), and with the spell prepared and no grant left.
func TestMR025_ALongCastWhoseGrantIsGoneIsRefusedAtTheFinish(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		levels []int32
		keep   int
		own    bool
	}{
		{"another grant of the spell is left", []int32{3, 5}, 1, true},
		{"the spell is not prepared and another grant is left", []int32{3, 5}, 1, false},
		{"the spell is prepared and no grant is left", []int32{4}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, feats := newClericGrants(t, tc.levels...)
			a.bri = a.briWithFeats(t, feats, tc.own) // started with every feat; the first goes
			id := a.startPrayer(t, feats[0])
			a.bri = a.briWithFeats(t, feats[1:1+tc.keep], tc.own)
			before := a.castState(t, tc.levels)
			_, err := a.finish(t, a.master, id)
			wantRefusal(t, "finish with the grant gone", err, connect.CodeInvalidArgument, "the grant the casting started through is gone")
			if after := a.castState(t, tc.levels); after != before {
				t.Errorf("the refused finish changed the state: %s, want %s", after, before)
			}
			if c := a.castByID(t, a.master, id); c.GetStatus() != playv1.OutsideCastStatus_OUTSIDE_CAST_STATUS_CASTING {
				t.Errorf("cast after the refused finish = %v, want it still being cast", c.GetStatus())
			}
			// The player can still give the casting up.
			if err := a.interrupt(t, a.bia, id); err != nil {
				t.Fatalf("abandon after the refused finish: %v", err)
			}
			if c := a.castByID(t, a.master, id); c.GetStatus() != playv1.OutsideCastStatus_OUTSIDE_CAST_STATUS_FAILED {
				t.Errorf("cast after the abandon = %v, want it failed", c.GetStatus())
			}
		})
	}
}

// What an abandoned long casting costs (characterisation, MR-025): a casting by a slot spends the slot only at the finish,
// so an abandon leaves it free (casting_effects.go spendCastSlot); a casting through a limited grant spends the use at
// the start (casting.go spendGrantUse) and an abandon does not give it back (casting_end.go closeCast takes back only
// what the cast did to its targets).
func TestMR025_AnAbandonedLongCastKeepsTheGrantUseAndSpendsNoSlot(t *testing.T) {
	t.Parallel()
	a, feats := newClericGrants(t, 4)
	bySlot := a.mustCastOut(t, a.bia, a.bri, prayerKey, &playv1.SpellSlot{Level: 2}, false, []*charactersv1.Character{a.toren}).GetCast().GetId()
	if err := a.interrupt(t, a.bia, bySlot); err != nil {
		t.Fatalf("abandon the casting by a slot: %v", err)
	}
	if got, want := a.castState(t, []int32{4}), "Toren 44 hp, slots used 0/0, usos_prece_4 used 0"; got != want {
		t.Errorf("after abandoning a casting by a slot: %s, want %s", got, want)
	}
	byGrant := a.startPrayer(t, feats[0])
	if err := a.interrupt(t, a.bia, byGrant); err != nil {
		t.Fatalf("abandon the casting through the grant: %v", err)
	}
	if got, want := a.castState(t, []int32{4}), "Toren 44 hp, slots used 0/0, usos_prece_4 used 1"; got != want {
		t.Errorf("after abandoning a casting through a grant: %s, want %s", got, want)
	}
}
