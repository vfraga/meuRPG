package play

import (
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// A table feature moves spell slots (MR-025): an action turns the uses it spends into a slot, an action spends a slot to
// heal by its level, and a recovery takes expended slots back after a short rest. Ragna is a cleric of level 5 (slots 4,
// 3 and 2) of an invented race. These tests need the database (MEURPG_TEST_DATABASE_URL).

// slotTotal is the slots of the level the vitals show, 0 for none.
func slotTotal(v *playv1.CharacterVitals, level int32) int32 {
	for _, s := range v.GetSpellSlots() {
		if s.GetLevel() == level {
			return s.GetTotal()
		}
	}
	return 0
}

// resourceLine is the resource entry of the log, as the user reads it.
func resourceLine(t *testing.T, a *armed, u *user, e *playv1.Encounter) *playv1.CombatLogResource {
	t.Helper()
	var out *playv1.CombatLogResource
	for _, r := range a.log(t, u, e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetActorLabel() == "Ragna" && en.GetResource() != nil {
				out = en.GetResource()
			}
		}
	}
	return out
}

func TestMR025_AnActionTurnsUsesIntoASpellSlotAndUndoes(t *testing.T) {
	t.Parallel()
	a := fxTableOf(t, "class:cleric", fxTrait("Centelha da Mesa",
		fxPool(poolKey, "3", "long_rest"),
		&rulesv1.TableEffect{Type: "grant_action", Economy: "bonus_action", Resource: poolKey, Cost: "2", GivesSlot: "2", TextPt: "Faísca de teste."},
	))
	e := fxFight(t, a)
	key := fxActionKey(t, a, e, "Centelha da Mesa")
	if o := fxOption(t, a, e, "Centelha da Mesa"); o.GetAction().GetGivesSlotLevel() != 2 || o.GetAction().GetCost() != 2 {
		t.Fatalf("the action = %v, want 2 uses for a slot of level 2", o.GetAction())
	}
	before := slotTotal(a.vitals(t, a.toren), 2)

	a.undoes(t, "the slot made from uses", func() {
		if _, err := fxUse(t, a, e, key, nil); err != nil {
			t.Fatalf("TakeAction() error = %v", err)
		}
		v := a.vitals(t, a.toren)
		if usedOf(v, poolKey) != 2 || slotTotal(v, 2) != before+1 || slotsUsed(v, 2) != 0 {
			t.Fatalf("after: %d uses spent, %d slots of level 2 (%d before), %d used; want 2, one more, none used", usedOf(v, poolKey), slotTotal(v, 2), before, slotsUsed(v, 2))
		}
		for name, u := range map[string]*user{"master": a.master, "Ragna's player": a.caio} {
			if r := resourceLine(t, a, u, e); r == nil || r.GetSlotLevel() != 2 || r.GetSpent() != 2 || r.GetKey() != key {
				t.Errorf("%s reads the line %v, want %s: 2 uses, a slot of level 2", name, r, key)
			}
		}
	})
	if v := a.vitals(t, a.toren); usedOf(v, poolKey) != 0 || slotTotal(v, 2) != before {
		t.Errorf("after the undo: %d uses spent, %d slots; want 0 and %d", usedOf(v, poolKey), slotTotal(v, 2), before)
	}

	// With fewer uses than the cost left the slot is not made.
	fxSpendOf(t, a, poolKey, 2)
	if _, err := fxUse(t, a, e, key, nil); err == nil {
		t.Error("a slot was made with 1 use left of the 2 it costs")
	}
	if v := a.vitals(t, a.toren); usedOf(v, poolKey) != 2 || slotTotal(v, 2) != before {
		t.Errorf("the refusal changed something: %d uses spent, %d slots", usedOf(v, poolKey), slotTotal(v, 2))
	}
}

func TestMR025_AnActionSpendsASpellSlotToHealByItsLevel(t *testing.T) {
	t.Parallel()
	a := fxTableOf(t, "class:cleric", fxTrait("Chama da Mesa",
		&rulesv1.TableEffect{Type: "grant_action", Economy: "action", SpendsSlot: true, Heal: "1d8", ValuePerPoint: "1d8", TextPt: "Cura de teste."},
	))
	e := fxFight(t, a)
	a.hurt(t, a.toren, 1)
	key := fxActionKey(t, a, e, "Chama da Mesa")
	if o := fxOption(t, a, e, "Chama da Mesa"); !o.GetAction().GetSpendsSlot() || o.GetAction().GetResourceKey() != "" || o.GetAction().GetHeal().GetSides() != 8 {
		t.Fatalf("the action = %v, want a spent slot that heals a d8", o.GetAction())
	}

	for name, n := range map[string]int32{"no level": 0, "level 10": 10, "a negative level": -1} {
		_, err := fxUse(t, a, e, key, fxSpend(n))
		wantFxCode(t, err, connect.CodeInvalidArgument, name)
	}
	// Nothing free at the level: the master's hand spent them both.
	a.correct(t, a.toren, func(r *playv1.AdjustCharacterVitalsRequest) {
		r.SpellSlotsUsed = []*playv1.SpellSlotsUsed{{Level: 3, Used: 2}}
	})
	_, err := fxUse(t, a, e, key, fxSpend(3))
	wantFxCode(t, err, connect.CodeFailedPrecondition, "a level with no free slot")
	_, err = fxUse(t, a, e, key, fxSpend(4))
	wantFxCode(t, err, connect.CodeFailedPrecondition, "a level the character has no slot of")
	if v := a.vitals(t, a.toren); v.GetHitPointsCurrent() != 1 || slotsUsed(v, 3) != 2 || byLabel(t, a.get(t, a.caio), "Ragna").GetActionUsed() {
		t.Fatalf("a refusal changed something: %d hit points, %d slots of level 3 used", v.GetHitPointsCurrent(), slotsUsed(v, 3))
	}

	a.undoes(t, "the heal by a slot", func() {
		a.h.roller.queue(4, 5)
		res, err := fxUse(t, a, e, key, fxSpend(2))
		if err != nil {
			t.Fatalf("TakeAction(2) error = %v", err)
		}
		v := a.vitals(t, a.toren)
		if v.GetHitPointsCurrent() != 10 || slotsUsed(v, 2) != 1 || res.GetHealed() != 9 || res.GetRoll().GetDiceCount() != 2 {
			t.Fatalf("after: %d hit points, %d slots of level 2 used, healed %d with %v; want 1 + 4 + 5 = 10, 1, 9 and 2d8", v.GetHitPointsCurrent(), slotsUsed(v, 2), res.GetHealed(), res.GetRoll())
		}
		for name, u := range map[string]*user{"master": a.master, "Ragna's player": a.caio} {
			if r := resourceLine(t, a, u, e); r == nil || r.GetSlotLevel() != 2 || r.Healed == nil || r.GetHealed() != 9 {
				t.Errorf("%s reads the line %v, want a slot of level 2 and 9 hit points", name, r)
			}
		}
		if r := resourceLine(t, a, a.bia, e); r != nil && r.Healed != nil {
			t.Errorf("Brisa's player reads %d hit points in the line of Ragna's action: only the master and the healed's player do", r.GetHealed())
		}
	})
}

// recoverFeature calls UseArcaneRecovery as u for a table feature's slot_recovery effect.
func (r *restTable) recoverFeature(t *testing.T, u *user, c *charactersv1.Character, feature string, slots map[int32]int32) (*playv1.UseArcaneRecoveryResponse, error) {
	t.Helper()
	req := &playv1.UseArcaneRecoveryRequest{CampaignId: r.campaign, CharacterId: c.GetId(), IdempotencyKey: newKey(), FeatureKey: feature}
	for level := int32(1); level <= 9; level++ {
		if n, ok := slots[level]; ok {
			req.Slots = append(req.Slots, &playv1.SpellSlotBack{Level: level, Count: n})
		}
	}
	res, err := u.resource.UseArcaneRecovery(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// A wizard of a table subclass whose feature takes back slots up to the 2nd level, of a combined level equal to the wizard level
// (3), once until a long rest. The wizard's own Arcane Recovery is a different resource and does not change.
func TestMR025_ATableFeatureRecoversSpellSlotsWithinItsLimits(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	res, err := r.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
		CampaignId: r.campaign, Body: &rulesv1.CreateTableEntryRequest_TableSubclass{TableSubclass: &rulesv1.TableSubclass{
			NamePt: "Escola do Retorno", ClassKey: "class:wizard", DescPt: []string{"Espaços de volta."},
			Levels: []*rulesv1.TableSubclassLevel{{Level: 2, Features: []*rulesv1.TableFeature{{NamePt: "Retorno", DescPt: []string{"Volta."}, Effects: []*rulesv1.TableEffect{
				fxPool("retorno", "1", "long_rest"),
				{Type: "slot_recovery", Resource: "retorno", RecoverMaxLevel: "2", RecoverLevels: "level()"},
			}}}}},
		}},
	}))
	if err != nil {
		t.Fatalf("CreateTableEntry() error = %v", err)
	}
	entry := res.Msg.GetEntry()
	feature := entry.GetTableSubclass().GetLevels()[0].GetFeatures()[0].GetKey()
	scores := abilities(10, 14, 14, 10, 10)
	scores.Intelligence = 16
	mage := r.ana.heroWith(t, r.campaign, "Elora", classLevel("class:wizard", 3, entry.GetKey()), scores, nil, nil, nil)
	r.expend(t, mage, map[int32]int32{1: 4, 2: 2})

	_, err = r.recoverFeature(t, r.ana, mage, feature, map[int32]int32{1: 1})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_SHORT_REST)
	r.shortRest(t)

	_, err = r.recoverFeature(t, r.ana, mage, "feature:nao-existe", map[int32]int32{1: 1})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NOT_AVAILABLE)
	_, err = r.recoverFeature(t, r.ana, r.fighter, feature, map[int32]int32{1: 1})
	wantCode(t, "Ana calls it on Bia's fighter", err, connect.CodePermissionDenied)
	_, err = r.recoverFeature(t, r.ana, mage, feature, map[int32]int32{3: 1})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_SLOT_LEVEL_TOO_HIGH)
	_, err = r.recoverFeature(t, r.ana, mage, feature, map[int32]int32{1: 2, 2: 1})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_OVER_ALLOWANCE)
	if v := r.vitals(t, r.ana, mage); usedOf(v, "retorno") != 0 || slotsUsed(v, 1) != 4 || slotsUsed(v, 2) != 2 {
		t.Fatalf("a refused call changed the vitals: uses spent %d, 1st used %d, 2nd used %d", usedOf(v, "retorno"), slotsUsed(v, 1), slotsUsed(v, 2))
	}

	got, err := r.recoverFeature(t, r.ana, mage, feature, map[int32]int32{2: 1, 1: 1})
	if err != nil {
		t.Fatalf("the recovery within the limits error = %v", err)
	}
	if v := got.GetVitals(); got.GetRecoveredLevels() != 3 || slotsUsed(v, 1) != 3 || slotsUsed(v, 2) != 1 || usedOf(v, "retorno") != 1 || usedOf(v, "arcane_recovery") != 0 {
		t.Errorf("after: %d levels, 1st used %d, 2nd used %d, retorno spent %d, arcane recovery spent %d; want 3, 3, 1, 1 and 0",
			got.GetRecoveredLevels(), slotsUsed(v, 1), slotsUsed(v, 2), usedOf(v, "retorno"), usedOf(v, "arcane_recovery"))
	}
	_, err = r.recoverFeature(t, r.ana, mage, feature, map[int32]int32{1: 1})
	wantResourceBlocked(t, err, playv1.ResourceBlockedReason_RESOURCE_BLOCKED_REASON_NO_USES_LEFT)
}

// A table subclass that changes Wild Shape and recovers slots stays on the sheets that have it when the master archives
// or switches it off (RN-23).
func TestMR025_AnArchivedOrSwitchedOffTableSubclassStaysOnItsSheet(t *testing.T) {
	t.Parallel()
	r := newRecoveryTable(t)
	res, err := r.master.table.CreateTableEntry(t.Context(), connect.NewRequest(&rulesv1.CreateTableEntryRequest{
		CampaignId: r.campaign, Body: &rulesv1.CreateTableEntryRequest_TableSubclass{TableSubclass: &rulesv1.TableSubclass{
			NamePt: "Círculo do Retorno", ClassKey: "class:druid", DescPt: []string{"Formas e espaços."},
			Levels: []*rulesv1.TableSubclassLevel{{Level: 2, Features: []*rulesv1.TableFeature{{NamePt: "Retorno", DescPt: []string{"Volta."}, Effects: []*rulesv1.TableEffect{
				fxPool("retorno", "1", "long_rest"),
				{Type: "wild_shape", MaxCr: "1", FormCost: "0"},
				{Type: "slot_recovery", Resource: "retorno", RecoverMaxLevel: "2", RecoverLevels: "2"},
			}}}}},
		}},
	}))
	if err != nil {
		t.Fatalf("CreateTableEntry() error = %v", err)
	}
	entry := res.Msg.GetEntry()
	feature := entry.GetTableSubclass().GetLevels()[0].GetFeatures()[0].GetKey()
	druid := r.ana.heroWith(t, r.campaign, "Sálvia", classLevel("class:druid", 5, entry.GetKey()), abilities(10, 12, 14, 10, 16), nil, nil, nil)
	r.expend(t, druid, map[int32]int32{1: 2})
	r.shortRest(t)

	maxCR := func() string {
		t.Helper()
		list, err := r.ana.characters.ListWildShapeForms(t.Context(), connect.NewRequest(&charactersv1.ListWildShapeFormsRequest{CampaignId: r.campaign, CharacterId: druid.GetId()}))
		if err != nil {
			t.Fatalf("ListWildShapeForms() error = %v", err)
		}
		return list.Msg.GetMaxCr()
	}
	if got := maxCR(); got != "1" {
		t.Fatalf("with the subclass the limit is CR %s, want 1", got)
	}
	if _, err := r.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: r.campaign, Key: entry.GetKey()})); err != nil {
		t.Fatalf("ArchiveTableEntry() error = %v", err)
	}
	if got := maxCR(); got != "1" {
		t.Errorf("after the archive the limit is CR %s, want 1: an archived entry still holds on the sheets that have it", got)
	}

	if _, err := r.master.table.UnarchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.UnarchiveTableEntryRequest{CampaignId: r.campaign, Key: entry.GetKey()})); err != nil {
		t.Fatalf("UnarchiveTableEntry() error = %v", err)
	}
	if _, err := r.master.table.SetOptionSwitches(t.Context(), connect.NewRequest(&rulesv1.SetOptionSwitchesRequest{
		CampaignId: r.campaign, Switches: []*rulesv1.OptionSwitch{{Key: entry.GetKey(), Off: true}},
	})); err != nil {
		t.Fatalf("SetOptionSwitches() error = %v", err)
	}
	if got := maxCR(); got != "1" {
		t.Errorf("after the switch-off the limit is CR %s, want 1: the sheet that has the subclass keeps it", got)
	}
	if _, err := r.recoverFeature(t, r.ana, druid, feature, map[int32]int32{1: 1}); err != nil {
		t.Errorf("the recovery after the switch-off error = %v, want none", err)
	}
}
