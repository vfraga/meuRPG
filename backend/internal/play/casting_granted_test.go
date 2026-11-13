package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The spells a granted_spell effect gives, cast outside a combat and in one (MR-025). The feat
// is the master's and its names are invented; the spells are the SRD's. These tests need the
// database (MEURPG_TEST_DATABASE_URL).

const (
	graceUses  = "usos_da_graca"
	lightSpell = "spell:light"
)

// newGraceParty is newCasters with a table feat the wizard has: Bênção (Bless, once a long rest,
// Charisma), and Luz (Light, at will, Intelligence), both with no slot.
func newGraceParty(t *testing.T) (a *armed, feat string) {
	t.Helper()
	return newGracePartyWith(t, []*rulesv1.TableEffect{
		{Type: "resource", Resource: graceUses, Max: "1", Recharge: "long_rest"},
		{Type: "granted_spell", Spell: blessKey, Ability: "cha", Uses: graceUses},
		{Type: "granted_spell", Spell: lightSpell, Ability: "int"},
	})
}

// newGracePartyWith is newGraceParty with the effects of the feat the wizard has.
func newGracePartyWith(t *testing.T, effects []*rulesv1.TableEffect) (a *armed, feat string) {
	t.Helper()
	a = newArmedWith(t, func(a *armed) {
		res, err := a.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
			CampaignId: a.campaignID, Body: &rulesv1.CreateTableEntryRequest_TableFeat{TableFeat: &rulesv1.TableFeat{
				NamePt: "Graça de teste", DescPt: []string{"Um dom de teste."},
				Effects: effects,
			}},
		}))
		if err != nil {
			t.Fatalf("CreateTableEntry(feat) error = %v", err)
		}
		feat = res.Msg.GetEntry().GetKey()
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 14},
			RaceKey:    "race:gnome", Classes: []*charactersv1.ClassLevel{{ClassKey: "class:wizard", Level: 5}},
			CantripKeys: []string{fireBolt}, KnownSpellKeys: []string{magicMissileSpell, shieldSpell}, PreparedSpellKeys: []string{magicMissileSpell, shieldSpell},
		}}}
		made, err := a.ana.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: a.campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus", Sheet: sheet,
		}))
		if err != nil {
			t.Fatalf("CreateCharacter(Pensantus) error = %v", err)
		}
		// A player's feats come from the level-up: the master adds the feat to the sheet (MR-025).
		sheet.GetFull().FeatKeys = []string{feat}
		upd, err := a.master.characters.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
			CampaignId: a.campaignID, CharacterId: made.Msg.GetCharacter().GetId(), Revision: made.Msg.GetCharacter().GetRevision(), Name: "Pensantus", Sheet: sheet,
		}))
		if err != nil {
			t.Fatalf("UpdateCharacter(Pensantus gets the feat) error = %v", err)
		}
		a.pens = upd.Msg.GetCharacter()
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil,
			[]string{cureWounds, healingWord, guidingBolt})
	})
	return a, feat
}

func grantedBy(feat string) func(*playv1.CastSpellOutsideCombatRequest) {
	return func(r *playv1.CastSpellOutsideCombatRequest) { r.GrantedBy = feat }
}

// A limited grant casts once with no slot and spends its use; the second cast is refused, and the
// slots were never touched. An at-will grant casts as often as the table likes.
func TestMR025_AGrantedSpellIsCastOutsideACombatWithNoSlot(t *testing.T) {
	t.Parallel()
	a, feat := newGraceParty(t)

	if _, err := a.castOut(t, a.ana, a.pens, blessKey, nil, false, []*charactersv1.Character{a.pens}, grantedBy(feat)); err != nil {
		t.Fatalf("CastSpellOutsideCombat(bless through the grant) error = %v", err)
	}
	used, total := resourceUsed(a.vitals(t, a.pens), graceUses)
	if used != 1 || total != 1 {
		t.Errorf("grant uses = %d of %d, want 1 of 1", used, total)
	}
	for _, sl := range a.vitals(t, a.pens).GetSpellSlots() {
		if sl.GetUsed() != 0 {
			t.Errorf("a slot of level %d was spent: %d", sl.GetLevel(), sl.GetUsed())
		}
	}

	_, err := a.castOut(t, a.ana, a.pens, blessKey, nil, false, []*charactersv1.Character{a.pens}, grantedBy(feat))
	wantCastingBlocked(t, "second cast of a once-only grant", err, playv1.CastingBlockedReason_CASTING_BLOCKED_REASON_NO_USES)
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 1 {
		t.Errorf("a refused cast spent a use: %d, want 1", used)
	}

	for range 3 {
		if _, err := a.castOut(t, a.ana, a.pens, lightSpell, nil, false, nil, grantedBy(feat)); err != nil {
			t.Fatalf("CastSpellOutsideCombat(light, at will) error = %v", err)
		}
	}
}

// Without the grant the wizard does not have the spell, and a name that is not a grant is refused.
func TestMR025_AGrantedSpellNeedsItsGrant(t *testing.T) {
	t.Parallel()
	a, feat := newGraceParty(t)
	if _, err := a.castOut(t, a.ana, a.pens, blessKey, nil, false, []*charactersv1.Character{a.pens}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("bless with no grant named: code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if _, err := a.castOut(t, a.ana, a.pens, blessKey, nil, false, []*charactersv1.Character{a.pens}, grantedBy(feat+"x")); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a grant that is not one: code = %v, want invalid_argument", connect.CodeOf(err))
	}
	// The grant gives nothing to a character that does not have the feat.
	if _, err := a.castOut(t, a.caio, a.toren, blessKey, nil, false, []*charactersv1.Character{a.toren}, grantedBy(feat)); err == nil {
		t.Error("a fighter without the feat cast the granted spell")
	}
	// With a slot, the grant must allow one.
	if _, err := a.castOut(t, a.ana, a.pens, blessKey, slotOfLevel(2), false, []*charactersv1.Character{a.pens}, grantedBy(feat)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a slot through a grant that allows none: code = %v, want invalid_argument", connect.CodeOf(err))
	}
}

// The listing of the spells says which ones come from a grant, with the uses left.
func TestMR025_TheCastableSpellsListTheGrants(t *testing.T) {
	t.Parallel()
	a, feat := newGraceParty(t)
	res, err := a.ana.casting.GetCastOptions(t.Context(), connect.NewRequest(&playv1.GetCastOptionsRequest{CampaignId: a.campaignID, CharacterId: a.pens.GetId()}))
	if err != nil {
		t.Fatalf("GetCastOptions() error = %v", err)
	}
	var step *playv1.CastingSpell
	for _, s := range res.Msg.GetSpells() {
		if s.GetSpell().GetKey() == blessKey {
			step = s
		}
	}
	if step == nil || len(step.GetGrants()) != 1 {
		t.Fatalf("bless = %+v, want one grant", step)
	}
	g := step.GetGrants()[0]
	if g.GetSourceKey() != feat || g.GetAtWill() || g.GetResource() != graceUses || g.GetUsesLeft() != 1 || !g.GetReady() || g.GetAbility() != "cha" {
		t.Errorf("grant = %+v, want the feat's, limited to its resource, 1 use left, Charisma", g)
	}
	if step.GetCanCast() {
		t.Error("the wizard can cast bless the class's way: it is not on his list")
	}
}

// In a combat, the granted spell is a turn option of its own: cast through the grant it takes no slot,
// and spends the use; a second cast is refused, the undo gives the use back.
func TestMR025_AGrantedSpellInCombat(t *testing.T) {
	t.Parallel()
	a, feat := newGraceParty(t)
	e := a.castersFight(t, 1)

	o := a.mustOptions(t, a.ana, e, "Pensantus")
	var opt *rulesv1.SpellOption
	for _, s := range o.GetOptions().GetSpells() {
		if s.GetSpell().GetKey() == blessKey {
			opt = s
		}
	}
	if opt == nil || opt.GetGrant().GetSourceKey() != feat || !opt.GetEnabled() || len(opt.GetSlots()) != 0 || opt.GetGrant().GetUsesLeft() != 1 {
		t.Fatalf("bless option = %+v, want an enabled grant with no slots and 1 use", opt)
	}

	granted := func(r *playv1.CastSpellRequest) { r.GrantedBy = feat; noCastRoll(r) }
	if _, err := a.cast(t, a.ana, e, "Pensantus", blessKey, nil, a.at(t, "Pensantus", "Toren"), granted); err != nil {
		t.Fatalf("CastSpell(bless through the grant) error = %v", err)
	}
	if used, _ := resourceUsed(a.vitals(t, a.pens), graceUses); used != 1 {
		t.Errorf("grant uses = %d, want 1", used)
	}
	if _, err := a.cast(t, a.ana, e, "Pensantus", blessKey, nil, a.at(t, "Pensantus", "Toren"), granted); err == nil {
		t.Error("a second cast of a once-only grant was accepted")
	}
	// The class's way has no bless: naming no grant for it is refused.
	if _, err := a.cast(t, a.ana, e, "Pensantus", blessKey, nil, a.at(t, "Pensantus"), noCastRoll); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("bless with no grant named: code = %v, want invalid_argument", connect.CodeOf(err))
	}
}
