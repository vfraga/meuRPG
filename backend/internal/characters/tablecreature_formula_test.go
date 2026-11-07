package characters

import (
	"slices"
	"strings"
	"testing"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// A table creature whose numbers are formulas of its owner: an SRD wolf with 5 hit
// points per level, an armor class of 10 + proficiency + Dexterity, and a claw that hits with proficiency +
// Strength. These tests need the database (MEURPG_TEST_DATABASE_URL).

const (
	silverWolfKey = "creature:lobo-de-prata@mesa"
	clawKey       = silverWolfKey + "#garra-de-prata"
)

func silverWolf() *rulesv1.TableCreature {
	return &rulesv1.TableCreature{
		NamePt: "Lobo de Prata", BaseKey: "monster:wolf",
		HitPointsFormula: "5 * level()", ArmorClassFormula: `10 + prof() + mod("dex")`,
		Attacks: []*rulesv1.TableCreatureAttack{{
			NamePt: "Garra de Prata", Melee: true, DiceCount: 1, DiceSides: 6, DamageType: "damage-type:slashing",
			ToHit: `prof() + mod("str")`, DamageBonus: `mod("str")`,
		}},
	}
}

// creatureNumbers is what a combat reads of a creature: armor class, and the to-hit and damage bonus of the claw.
type creatureNumbers struct{ hp, ac, toHit, damageBonus int }

// numbersOf reads the creature through the list (hit points, armor class) and the roster's sheet (the claw).
func (tb *creatureTable) numbersOf(t *testing.T, u *user, owner *charactersv1.Character, id string) creatureNumbers {
	t.Helper()
	res, err := u.api.ListCharacterCreatures(t.Context(), connect.NewRequest(&charactersv1.ListCharacterCreaturesRequest{CampaignId: tb.campaign, CharacterId: owner.GetId()}))
	if err != nil {
		t.Fatalf("ListCharacterCreatures() error = %v", err)
	}
	var out creatureNumbers
	found := false
	for _, c := range res.Msg.GetCreatures() {
		if c.GetId() == id {
			out.hp, out.ac, found = int(c.GetHitPointsMax()), int(c.GetArmorClass()), true
		}
	}
	if !found {
		t.Fatalf("creature %s is not listed", id)
	}
	var sheet link.Sheet
	err = db.InTx(t.Context(), tb.h.pool, func(tx pgx.Tx) error {
		var ok bool
		var err error
		sheet, ok, err = tb.h.svc.CreatureSheet(t.Context(), tx, tb.campaign, id, silverWolfKey, "full")
		if err == nil && !ok {
			t.Fatal("CreatureSheet() does not know the creature")
		}
		return err
	})
	if err != nil {
		t.Fatalf("CreatureSheet() error = %v", err)
	}
	if sheet.ArmorClass != out.ac {
		t.Errorf("the sheet's armor class %d differs from the list's %d", sheet.ArmorClass, out.ac)
	}
	i := slices.IndexFunc(sheet.Attacks, func(a link.Attack) bool { return a.Key == clawKey })
	if i < 0 {
		t.Fatalf("the sheet's attacks %v lack the claw", sheet.Attacks)
	}
	out.toHit, out.damageBonus = sheet.Attacks[i].ToHit, sheet.Attacks[i].DiceBonus
	return out
}

func (tb *creatureTable) statBlockOf(t *testing.T, id string) []byte {
	t.Helper()
	var raw []byte
	if err := tb.h.pool.QueryRow(t.Context(), "SELECT stat_block FROM character_creatures WHERE id = $1", id).Scan(&raw); err != nil {
		t.Fatalf("read the stat block: %v", err)
	}
	return raw
}

func TestMR578_AFormulaCreatureHasTheNumbersOfItsOwner(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	fighter := tb.other.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren", levelUpFighterSheet(5, 6500, ""))
	tb.master.addEntry(t, tb.campaign, silverWolf())
	tb.master.addEntry(t, tb.campaign, tableWolf())

	give := func(c *charactersv1.Character, key string) *charactersv1.CharacterCreature {
		t.Helper()
		got, err := tb.give(tb.master, &charactersv1.GiveCreatureRequest{CharacterId: c.GetId(), MonsterKey: key})
		if err != nil {
			t.Fatalf("GiveCreature(%s) error = %v", key, err)
		}
		return got
	}
	wizardWolf, fighterWolf := give(tb.pc, silverWolfKey), give(fighter, silverWolfKey)

	// Pensantus: level 3, proficiency +2, Dexterity 16 (+3), Strength 12 (+1).
	want := creatureNumbers{hp: 15, ac: 15, toHit: 3, damageBonus: 1}
	if got := tb.numbersOf(t, tb.owner, tb.pc, wizardWolf.GetId()); got != want {
		t.Errorf("the wizard's wolf = %+v, want %+v", got, want)
	}
	// Toren: level 5, proficiency +3, Dexterity 13 (+1), Strength 16 (+3).
	wantFighter := creatureNumbers{hp: 25, ac: 14, toHit: 6, damageBonus: 3}
	if got := tb.numbersOf(t, tb.other, fighter, fighterWolf.GetId()); got != wantFighter {
		t.Errorf("the fighter's wolf = %+v, want %+v", got, wantFighter)
	}
	if wizardWolf.GetHitPointsMax() != 15 || wizardWolf.GetHitPointsCurrent() != 15 || fighterWolf.GetHitPointsMax() != 25 {
		t.Errorf("the gifts' hit points = %d/%d and %d, want 15/15 and 25", wizardWolf.GetHitPointsCurrent(), wizardWolf.GetHitPointsMax(), fighterWolf.GetHitPointsMax())
	}
	// The master reads the same numbers.
	if got := tb.numbersOf(t, tb.master, tb.pc, wizardWolf.GetId()); got != want {
		t.Errorf("the master reads %+v, want %+v", got, want)
	}

	// The snapshot is kept: the owner levels up and the master edits the formulas, and the creature stays.
	up := pensantusSheet()
	up.GetFull().Classes[0].Level = 5
	up.GetFull().ExperiencePoints = 6500
	if _, err := tb.master.update(t, tb.master.get(t, tb.campaign, tb.pc.GetId()), "Pensantus", up); err != nil {
		t.Fatalf("raising the owner's level: %v", err)
	}
	entry := tb.master.entries(t, tb.campaign)[silverWolfKey]
	next := silverWolf()
	next.HitPointsFormula, next.ArmorClassFormula = "100", "30"
	next.Attacks[0].ToHit, next.Attacks[0].DamageBonus = "9", "9"
	if _, err := tb.master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(tb.campaign, silverWolfKey, entry.GetRevision(), next))); err != nil {
		t.Fatalf("UpdateTableEntry() error = %v", err)
	}
	if got := tb.numbersOf(t, tb.owner, tb.pc, wizardWolf.GetId()); got != want {
		t.Errorf("after the owner's level-up and the edit: %+v, want the snapshot %+v", got, want)
	}
	// A creature given now has the new formulas and the new level.
	later := give(tb.pc, silverWolfKey)
	if got := tb.numbersOf(t, tb.owner, tb.pc, later.GetId()); got != (creatureNumbers{hp: 100, ac: 30, toHit: 9, damageBonus: 9}) {
		t.Errorf("a creature given after the edit = %+v, want the new formulas", got)
	}

	// A table creature with no formula keeps no snapshot; an SRD creature neither.
	static := give(tb.pc, tableWolfKey)
	if raw := tb.statBlockOf(t, static.GetId()); raw != nil {
		t.Errorf("a creature with no formula has a snapshot: %s", raw)
	}
	if raw := tb.statBlockOf(t, tb.wolf(t, "Comum").GetId()); raw != nil {
		t.Errorf("an SRD creature has a snapshot: %s", raw)
	}
	if raw := tb.statBlockOf(t, wizardWolf.GetId()); !strings.Contains(string(raw), `"HitPoints": 15`) {
		t.Errorf("the snapshot = %s, want the resolved numbers", raw)
	}
}

func TestMR578_TheFormulaFieldsOfACreatureHaveTheirPaths(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Samuel"), h.newUser("Dona")
	campaign := h.newCampaign(master, "Mirathel", player)
	bad := silverWolf()
	bad.Attacks = append(bad.Attacks, bad.Attacks[0], &rulesv1.TableCreatureAttack{
		NamePt: "Mordida", Melee: true, DiceCount: 1, DiceSides: 4, DamageType: "damage-type:piercing", ToHit: "prof() +",
	})
	bad.Attacks[1] = &rulesv1.TableCreatureAttack{NamePt: "Pancada", Melee: true, DiceCount: 1, DiceSides: 4, DamageType: "damage-type:bludgeoning"}
	bad.ArmorClassFormula = "wisdom()"
	_, err := master.table.CreateTableEntry(t.Context(), connect.NewRequest(createReq(campaign, bad)))
	fields := map[string]string{}
	for _, v := range violationsOfErr(t, err) {
		fields[v.GetField()] = v.GetReason()
	}
	if fields["table_creature.attacks[2].to_hit"] != rules.ReasonFormula || fields["table_creature.armor_class_formula"] != rules.ReasonFormula || len(fields) != 2 {
		t.Errorf("violations = %v, want a formula at table_creature.attacks[2].to_hit and at table_creature.armor_class_formula", fields)
	}
}

func TestMR578_AFormulaCreatureTravelsInAPackWithEveryField(t *testing.T) {
	t.Parallel()
	pt := newPackTable(t)
	full := silverWolf()
	full.DamageBonusFormula, full.AttackBonusFormula = "1", "prof()"
	full.HitPoints, full.ArmorClass, full.DamageBonus = 30, 14, 2
	full.Attacks = append(full.Attacks, &rulesv1.TableCreatureAttack{
		NamePt: "Espinho", Melee: false, RangeFt: 30, LongRangeFt: 90, DiceCount: 2, DiceSides: 8, DamageType: "damage-type:piercing",
		ToHit: "slot() + prof()", DamageBonus: "slot()",
	})
	made := pt.master.addEntry(t, pt.from, full)
	pack := pt.export(t, pt.from)
	file, err := storeJSON.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	loaded := &rulesv1.TableContentPack{}
	if err := loadJSON.Unmarshal(file, loaded); err != nil {
		t.Fatal(err)
	}
	pt.apply(t, pt.to, loaded, "formula-1")
	in := pt.entries(t, pt.to)[silverWolfKey]
	if in == nil || !proto.Equal(bodyMessage(in), bodyMessage(made)) {
		t.Fatalf("the imported creature = %v, want the exported one", in)
	}
	got := bodyMessage(in).(*rulesv1.TableCreature)
	if got.GetAttackBonusFormula() != "prof()" || len(got.GetAttacks()) != 2 || got.GetAttacks()[1].GetLongRangeFt() != 90 || got.GetAttacks()[1].GetToHit() != "slot() + prof()" {
		t.Errorf("the imported creature lost a field: %v", got)
	}
}

// giveTo gives the creature to the character, or fails the test.
func (tb *creatureTable) giveTo(t *testing.T, c *charactersv1.Character, key string) *charactersv1.CharacterCreature {
	t.Helper()
	got, err := tb.give(tb.master, &charactersv1.GiveCreatureRequest{CharacterId: c.GetId(), MonsterKey: key})
	if err != nil {
		t.Fatalf("GiveCreature(%s) error = %v", key, err)
	}
	return got
}

func (tb *creatureTable) edit(t *testing.T, key string, body *rulesv1.TableCreature) {
	t.Helper()
	entry := tb.master.entries(t, tb.campaign)[key]
	if _, err := tb.master.table.UpdateTableEntry(t.Context(), connect.NewRequest(updateReq(tb.campaign, key, entry.GetRevision(), body))); err != nil {
		t.Fatalf("UpdateTableEntry(%s) error = %v", key, err)
	}
}

func TestMR578_ASnapshotIsUsedWheneverTheRowHasOneAndSurvivesAnEditOfTheAttacks(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	tb.master.addEntry(t, tb.campaign, silverWolf())
	wolf := tb.giveTo(t, tb.pc, silverWolfKey)
	want := creatureNumbers{hp: 15, ac: 15, toHit: 3, damageBonus: 1}
	if got := tb.numbersOf(t, tb.owner, tb.pc, wolf.GetId()); got != want {
		t.Fatalf("the gift = %+v, want %+v", got, want)
	}

	// The master adds an attack in front of the claw and gives the claw other formulas: the claw keeps its numbers.
	next := silverWolf()
	next.HitPointsFormula, next.ArmorClassFormula = "100", "30"
	next.Attacks = []*rulesv1.TableCreatureAttack{
		{NamePt: "Mordida de Prata", Melee: true, DiceCount: 1, DiceSides: 4, DamageType: "damage-type:piercing", ToHit: "9"},
		{NamePt: "Garra de Prata", Melee: true, DiceCount: 1, DiceSides: 6, DamageType: "damage-type:slashing", ToHit: "9", DamageBonus: "9"},
	}
	tb.edit(t, silverWolfKey, next)
	if got := tb.numbersOf(t, tb.owner, tb.pc, wolf.GetId()); got != want {
		t.Errorf("after an attack was added in front: %+v, want the snapshot %+v", got, want)
	}

	// The master takes every formula away: the list and the combat both still read the snapshot.
	plain := silverWolf()
	plain.HitPointsFormula, plain.ArmorClassFormula = "", ""
	plain.HitPoints, plain.ArmorClass = 7, 11
	plain.Attacks[0].ToHit, plain.Attacks[0].DamageBonus = "", ""
	tb.edit(t, silverWolfKey, plain)
	if got := tb.numbersOf(t, tb.owner, tb.pc, wolf.GetId()); got != want {
		t.Errorf("after the entry lost its formulas: %+v, want the snapshot %+v", got, want)
	}
	// A creature given now has the entry's static numbers (and no snapshot).
	fresh := tb.giveTo(t, tb.pc, silverWolfKey)
	if raw := tb.statBlockOf(t, fresh.GetId()); raw != nil {
		t.Errorf("a creature of an entry with no formula has a snapshot: %s", raw)
	}
}

func TestMR578_TheShieldAndTheActiveStatesOfTheOwnerReachTheFormulas(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	feature := tb.master.addEntry(t, tb.campaign, stanceSubclass(
		&rulesv1.TableEffect{Type: "resource", Resource: "posturas", Max: "1", Recharge: "short_rest"},
		&rulesv1.TableEffect{Type: "state", StateName: "postura", DurationRounds: "10", EndsOn: []string{"dismiss"}, Resource: "posturas", Economy: "bonus_action"},
		&rulesv1.TableEffect{Type: "modifier", Target: "speed.walk", Mode: "add", Value: "10", When: `active("postura")`},
	))
	stateKey := feature.GetTableSubclass().GetLevels()[0].GetFeatures()[0].GetKey() + "#postura"
	body := silverWolf()
	body.HitPointsFormula, body.ArmorClassFormula = "speedwalk()", `10 + (shield() ? 5 : 0)`
	tb.master.addEntry(t, tb.campaign, body)

	// One fighter (a player has one character) whose shield, weapon and states change between the gifts.
	hero := tb.other.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren", levelUpFighterSheet(5, 6500, feature.GetKey()))
	read := func(shield bool, weapon string) creatureNumbers {
		t.Helper()
		sheet := levelUpFighterSheet(5, 6500, feature.GetKey())
		sheet.GetFull().Shield = shield
		sheet.GetFull().WeaponKeys = []string{weapon}
		if _, err := tb.master.update(t, tb.master.get(t, tb.campaign, hero.GetId()), "Toren", sheet); err != nil {
			t.Fatalf("changing the fighter's gear: %v", err)
		}
		return tb.numbersOf(t, tb.other, hero, tb.giveTo(t, hero, silverWolfKey).GetId())
	}
	bare := read(false, "equipment:longsword")
	shielded := read(true, "equipment:longsword")
	idle := read(true, "equipment:greatsword")
	if bare.ac != 10 || shielded.ac != 15 || idle.ac != 10 {
		t.Errorf("armor class without a shield, with one and with one a two-handed weapon idles = %d, %d, %d; want 10, 15, 10", bare.ac, shielded.ac, idle.ac)
	}

	// With the state on the walk is faster, which the formula reads.
	if _, err := tb.h.pool.Exec(t.Context(), `UPDATE characters SET active_states = $2::jsonb WHERE id = $1`, hero.GetId(), `[{"key":"`+stateKey+`","rounds_left":5}]`); err != nil {
		t.Fatalf("turning the state on: %v", err)
	}
	if moving := read(false, "equipment:longsword"); moving.hp != bare.hp+10 {
		t.Errorf("hit points = speed: %d with the state off, %d on; want 10 more", bare.hp, moving.hp)
	}
}

func TestMR578_AMalformedSnapshotFallsBackToTheContentOrIsClamped(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	tb.master.addEntry(t, tb.campaign, silverWolf())
	wolf := tb.giveTo(t, tb.pc, silverWolfKey)
	// The content's block, for an owner with no formula run: the wolf's armor class 13 and a claw with +0 to hit.
	content := creatureNumbers{hp: 15, ac: 13, toHit: 0, damageBonus: 0}
	cases := []struct {
		name, raw string
		want      creatureNumbers
	}{
		{"an array", `[1, 2]`, content},
		{"a string", `"text"`, content},
		{"a number", `7`, content},
		{"an empty object", `{}`, content},
		{"another creature's key", `{"Key": "creature:outra@mesa", "ArmorClass": 30}`, content},
		{"a field of the wrong type", `{"Key": "` + silverWolfKey + `", "ArmorClass": "alto"}`, content},
		{"numbers out of range", `{"Key": "` + silverWolfKey + `", "ArmorClass": -3, "HitPoints": 99999999}`, creatureNumbers{hp: 15, ac: 1, toHit: 0, damageBonus: 0}},
		{
			"forged attack keys", `{"Key": "` + silverWolfKey + `", "ArmorClass": 20, "Attacks": [{"Key": "x#y", "AttackBonus": 20}, {"Key": "` + clawKey + `", "AttackBonus": 500}, {"Key": "` + clawKey + `", "AttackBonus": 2}]}`,
			creatureNumbers{hp: 15, ac: 20, toHit: rules.MaxCreatureAttackBonus, damageBonus: 0},
		},
	}
	for _, tc := range cases {
		if _, err := tb.h.pool.Exec(t.Context(), `UPDATE character_creatures SET stat_block = $2::jsonb WHERE id = $1`, wolf.GetId(), tc.raw); err != nil {
			t.Fatalf("%s: writing the row: %v", tc.name, err)
		}
		if got := tb.numbersOf(t, tb.owner, tb.pc, wolf.GetId()); got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// A creature whose row is gone (its owner's character was deleted in the middle of a read) has no snapshot: the sheet
// falls back to the content's static numbers and does not fail, whether its entry has formulas or none.
func TestMR578_ACreatureWithoutARowFallsBackToTheStaticNumbers(t *testing.T) {
	t.Parallel()
	plain := &rulesv1.TableCreature{NamePt: "Cão Simples", BaseKey: "monster:wolf", ArmorClass: 17}
	for _, tc := range []struct {
		name   string
		entry  *rulesv1.TableCreature
		key    string
		wantAC int
	}{
		{"an entry with formulas", silverWolf(), silverWolfKey, 13},
		{"an entry with no formula", plain, "creature:cao-simples@mesa", 17},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tb := newCreatureTable(t)
			tb.master.addEntry(t, tb.campaign, tc.entry)
			var sheet link.Sheet
			var ok bool
			err := db.InTx(t.Context(), tb.h.pool, func(tx pgx.Tx) error {
				var err error
				sheet, ok, err = tb.h.svc.CreatureSheet(t.Context(), tx, tb.campaign, uuid.New().String(), tc.key, "full")
				return err
			})
			if err != nil || !ok {
				t.Fatalf("CreatureSheet() of a creature with no row = %v, %v, want its sheet", ok, err)
			}
			if sheet.ArmorClass != tc.wantAC {
				t.Errorf("armor class = %d, want the static %d", sheet.ArmorClass, tc.wantAC)
			}
			i := slices.IndexFunc(sheet.Attacks, func(a link.Attack) bool { return a.Key == clawKey })
			if tc.key == silverWolfKey && i < 0 {
				t.Fatalf("the sheet's attacks %v lack the claw", sheet.Attacks)
			}
			if i >= 0 && (sheet.Attacks[i].ToHit != 0 || sheet.Attacks[i].DiceBonus != 0) {
				t.Errorf("the claw = %+v, want the static +0", sheet.Attacks[i])
			}
		})
	}
}

// The stat block of a creature of a character is the one worked out for its owner when it arrived (the page of
// the creature reads it), not the entry's static numbers; only the master and the owner's player may ask for it.
func TestMR578_TheStatBlockOfACharactersCreatureIsTheOneWorkedOutForItsOwner(t *testing.T) {
	t.Parallel()
	tb := newCreatureTable(t)
	fighter := tb.other.create(t, tb.campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Toren", levelUpFighterSheet(5, 6500, ""))
	tb.master.addEntry(t, tb.campaign, silverWolf())
	give := func(c *charactersv1.Character) *charactersv1.CharacterCreature {
		t.Helper()
		got, err := tb.give(tb.master, &charactersv1.GiveCreatureRequest{CharacterId: c.GetId(), MonsterKey: silverWolfKey})
		if err != nil {
			t.Fatalf("GiveCreature() error = %v", err)
		}
		return got
	}
	wizardWolf, fighterWolf := give(tb.pc), give(fighter)
	read := func(u *user, key, id string) (*rulesv1.Creature, error) {
		res, err := u.content.GetCreature(t.Context(), connect.NewRequest(&rulesv1.GetCreatureRequest{CampaignId: tb.campaign, Key: key, CreatureId: id}))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetCreature(), nil
	}
	claw := func(c *rulesv1.Creature) int32 {
		for _, a := range c.GetActions() {
			if a.GetHasAttack() && strings.Contains(a.GetName(), "Garra") {
				return a.GetAttackBonus()
			}
		}
		t.Fatalf("the stat block has no claw: %v", c.GetActions())
		return 0
	}
	// Without the creature's id the entry has no owner: the static numbers (the base's armor class).
	plain, err := read(tb.owner, silverWolfKey, "")
	if err != nil {
		t.Fatalf("GetCreature() by key error = %v", err)
	}
	if plain.GetArmorClass() == 15 {
		t.Fatalf("the stat block by key has the owner's armor class %d; it has no owner", plain.GetArmorClass())
	}
	// Pensantus (proficiency +2, Dexterity +3, Strength +1): armor class 15, claw +3. Toren (+3, +1, +3): 14, +6.
	for _, tt := range []struct {
		name          string
		u             *user
		id            string
		ac, clawBonus int32
	}{
		{"the owner's player", tb.owner, wizardWolf.GetId(), 15, 3},
		{"the master, on the wizard's wolf (the positive control)", tb.master, wizardWolf.GetId(), 15, 3},
		{"the master, on the fighter's wolf", tb.master, fighterWolf.GetId(), 14, 6},
		{"the other player, on her own wolf", tb.other, fighterWolf.GetId(), 14, 6},
	} {
		got, err := read(tt.u, silverWolfKey, tt.id)
		if err != nil {
			t.Errorf("%s: GetCreature() error = %v", tt.name, err)
			continue
		}
		if got.GetArmorClass() != tt.ac || claw(got) != tt.clawBonus {
			t.Errorf("%s: armor class %d and claw +%d, want %d and +%d", tt.name, got.GetArmorClass(), claw(got), tt.ac, tt.clawBonus)
		}
	}
	for _, tt := range []struct {
		name, key, id string
		u             *user
	}{
		{"another player asks for a creature that is not hers", silverWolfKey, wizardWolf.GetId(), tb.other},
		{"a key that is not the creature's", "monster:wolf", wizardWolf.GetId(), tb.owner},
		{"an id that is not one", silverWolfKey, "not-an-id", tb.owner},
	} {
		if _, err := read(tt.u, tt.key, tt.id); connect.CodeOf(err) != connect.CodeNotFound {
			t.Errorf("%s: error = %v, want not_found", tt.name, err)
		}
	}
}
