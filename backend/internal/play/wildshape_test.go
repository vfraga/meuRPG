package play

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"uuid"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps"
	"github.com/PuraFome/meuRPG/backend/internal/platform/httpserver"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Wild Shape (MR-037, Etapa 9, slice 9.10): a druid becomes an SRD beast, in a
// combat or out of one. The fixture is the table of the designs: Toren (a fighter),
// Pensantus (a wizard with a familiar), Sálvia, a level 5 druid (38 hit points,
// Wild Shape up to challenge rating 1/2 without a fly speed), and Irmã, a cleric
// who heals. These tests need the database (MEURPG_TEST_DATABASE_URL).

const (
	wolfKey  = "monster:wolf"
	wolfBite = "monster:wolf#bite"
)

// shapers is the table of this file.
type shapers struct {
	*armed
	dani *user // Irmã's player
	irma *charactersv1.Character
}

// newShapers builds the table: Sálvia is Bia's (a.bri), Pensantus Ana's, Toren Caio's
// and Irmã Dani's.
func newShapers(t *testing.T) *shapers {
	t.Helper()
	var irma *charactersv1.Character
	var dani *user
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 9,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{findFamiliar, animateDead, magicMissileSpell}, []string{findFamiliar, animateDead, magicMissileSpell})
		a.bri = a.bia.caster(t, a.campaignID, "Sálvia", "class:druid", "race:half-elf", 5,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 12, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, nil, nil, nil,
			[]string{conjureAnimals})
		dani = a.h.newUser("Dani")
		a.h.join(a.master, a.campaignID, dani)
		irma = dani.caster(t, a.campaignID, "Irmã", "class:cleric", "race:human", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil,
			[]string{cureWounds})
	})
	return &shapers{armed: a, dani: dani, irma: irma}
}

// fight starts the combat: Sálvia first, with Irmã next to her and the Capitão and a
// goblin on her other sides; Pensantus and Toren far away.
func (s *shapers) fight(t *testing.T) *playv1.Encounter {
	t.Helper()
	return s.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: s.capitao.GetId()}, {CharacterId: s.goblin.GetId()}},
		npcRolls: []int{1, 1},
		players:  map[string]int32{"Sálvia": 20, "Irmã": 15, "Pensantus": 12, "Toren": 10},
		reveal:   []string{"Capitão Goblin", "Goblin"},
		at: map[string][2]int32{
			"Sálvia": {6, 5}, "Irmã": {7, 5}, "Capitão Goblin": {5, 5}, "Goblin": {6, 6}, "Pensantus": {12, 2}, "Toren": {2, 2},
		},
	})
}

// assume calls AssumeWildShape as u.
func (a *armed) assume(t *testing.T, u *user, c *charactersv1.Character, beast string) (*playv1.AssumeWildShapeResponse, error) {
	t.Helper()
	res, err := u.play.AssumeWildShape(t.Context(), connect.NewRequest(&playv1.AssumeWildShapeRequest{
		CampaignId: a.campaignID, CharacterId: c.GetId(), BeastKey: beast, IdempotencyKey: newKey(),
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (a *armed) mustAssume(t *testing.T, u *user, c *charactersv1.Character, beast string) *playv1.AssumeWildShapeResponse {
	t.Helper()
	res, err := a.assume(t, u, c, beast)
	if err != nil {
		t.Fatalf("AssumeWildShape(%s) error = %v", beast, err)
	}
	return res
}

// leave calls LeaveWildShape as u.
func (a *armed) leave(t *testing.T, u *user, c *charactersv1.Character) (*playv1.LeaveWildShapeResponse, error) {
	t.Helper()
	res, err := u.play.LeaveWildShape(t.Context(), connect.NewRequest(&playv1.LeaveWildShapeRequest{
		CampaignId: a.campaignID, CharacterId: c.GetId(), IdempotencyKey: newKey(),
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (a *armed) mustLeave(t *testing.T, u *user, c *charactersv1.Character) *playv1.LeaveWildShapeResponse {
	t.Helper()
	res, err := a.leave(t, u, c)
	if err != nil {
		t.Fatalf("LeaveWildShape() error = %v", err)
	}
	return res
}

// used is how many uses of the Forma Selvagem resource are spent.
func used(v *playv1.CharacterVitals, key string) int32 {
	for _, r := range v.GetResources() {
		if r.GetKey() == key {
			return r.GetUsed()
		}
	}
	return -1
}

// lastPayload is the payload of the session's latest event of a kind, as JSON.
func (a *armed) lastPayload(t *testing.T, kind string) map[string]any {
	t.Helper()
	var body []byte
	if err := a.h.pool.QueryRow(t.Context(), `SELECT payload FROM session_events WHERE kind = $1 ORDER BY seq DESC LIMIT 1`, kind).Scan(&body); err != nil {
		t.Fatalf("read the latest %s event: %v", kind, err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode the %s payload %s: %v", kind, body, err)
	}
	return out
}

// TestMR037_WildShapeBeastsByLevel: the beasts the druid may become follow its level
// (level 2: challenge rating up to 1/4, no fly or swim speed; 4: up to 1/2, no fly;
// 8: up to 1), from the rules engine's list; a character that is not a druid has none.
func TestMR037_WildShapeBeastsByLevel(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	scores := &rulesv1.AbilityScores{Strength: 10, Dexterity: 12, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}
	owners := map[string]*user{}
	// A player has one living character in a campaign: each druid has a player.
	druid := func(name string, level int32) *charactersv1.Character {
		u := s.h.newUser("Jogador " + name)
		s.h.join(s.master, s.campaignID, u)
		c := u.hero(t, s.campaignID, name, "class:druid", "race:human", level, scores, nil, nil)
		owners[c.GetId()] = u
		return c
	}
	forms := func(u *user, c *charactersv1.Character) (*charactersv1.ListWildShapeFormsResponse, error) {
		res, err := u.characters.ListWildShapeForms(t.Context(), connect.NewRequest(&charactersv1.ListWildShapeFormsRequest{CampaignId: s.campaignID, CharacterId: c.GetId()}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	keysOf := func(r *charactersv1.ListWildShapeFormsResponse) []string {
		var out []string
		for _, f := range r.GetForms() {
			out = append(out, f.GetKey())
		}
		return out
	}
	// What the rules engine says a creature list with the same limits holds.
	want := func(maxCR string, noFly, noSwim bool) []string {
		res, err := s.bia.content.ListCreatures(t.Context(), connect.NewRequest(&rulesv1.ListCreaturesRequest{
			CampaignId: s.campaignID, Type: "beast", MaxCr: maxCR, NoFly: noFly, NoSwim: noSwim, PageSize: 100,
		}))
		if err != nil {
			t.Fatalf("ListCreatures() error = %v", err)
		}
		var out []string
		for _, c := range res.Msg.GetCreatures() {
			out = append(out, c.GetKey())
		}
		return out
	}

	for _, tc := range []struct {
		level          int32
		maxCR          string
		noFly, noSwim  bool
		in, out        []string
		wantCountExact int
	}{
		{2, "1/4", true, true, []string{wolfKey, "monster:mastiff"}, []string{"monster:crocodile", "monster:bat", "monster:brown-bear"}, 0},
		{5, "1/2", true, false, []string{wolfKey, "monster:crocodile"}, []string{"monster:bat", "monster:brown-bear", "monster:giant-eagle"}, 48},
		{8, "1", false, false, []string{wolfKey, "monster:brown-bear", "monster:giant-eagle"}, []string{"monster:giant-boar", "monster:giant-constrictor-snake"}, 0},
	} {
		c := druid("Folha", tc.level)
		got, err := forms(owners[c.GetId()], c)
		if err != nil {
			t.Fatalf("ListWildShapeForms(level %d) error = %v", tc.level, err)
		}
		if got.GetMaxCr() != tc.maxCR || got.GetNoFly() != tc.noFly || got.GetNoSwim() != tc.noSwim {
			t.Errorf("level %d: limit = %q, no fly %v, no swim %v; want %q, %v, %v", tc.level, got.GetMaxCr(), got.GetNoFly(), got.GetNoSwim(), tc.maxCR, tc.noFly, tc.noSwim)
		}
		if g, w := keysOf(got), want(tc.maxCR, tc.noFly, tc.noSwim); !slices.Equal(slices.Sorted(slices.Values(g)), slices.Sorted(slices.Values(w))) {
			t.Errorf("level %d: forms = %d beasts, the engine's list has %d", tc.level, len(g), len(w))
		}
		for _, k := range tc.in {
			if !slices.Contains(keysOf(got), k) {
				t.Errorf("level %d: %s is not in the list, want it", tc.level, k)
			}
		}
		for _, k := range tc.out {
			if slices.Contains(keysOf(got), k) {
				t.Errorf("level %d: %s is in the list, want it left out", tc.level, k)
			}
		}
		if tc.wantCountExact != 0 && len(got.GetForms()) != tc.wantCountExact {
			t.Errorf("level %d: %d beasts, want %d (E9-11)", tc.level, len(got.GetForms()), tc.wantCountExact)
		}
	}
	// A level 1 druid and a fighter have none.
	broto := druid("Broto", 1)
	for _, c := range []*charactersv1.Character{broto, s.toren} {
		u := owners[c.GetId()]
		if c == s.toren {
			u = s.caio
		}
		got, err := forms(u, c)
		if err != nil || len(got.GetForms()) != 0 || got.GetMaxCr() != "" {
			t.Errorf("%s: forms = %v, %v, want none", c.GetName(), got, err)
		}
	}
}

// TestMR037_WildShapeInCombat: Sálvia becomes a wolf in her turn. It costs the action
// and one use; the combatant takes the beast's speed and size, her armor class is the
// wolf's, her turn shows the wolf's Bite and no spells, and spells are refused with a
// typed reason. The beast's hit points are the master's and her own player's alone.
func TestMR037_WildShapeInCombat(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	e := s.fight(t)
	own := s.vitals(t, s.bri)
	if own.GetHitPointsMax() != 38 || used(own, wildShapeResource) != 0 {
		t.Fatalf("Sálvia = %v, want 38 PV and no use spent", own)
	}
	before := byLabel(t, e, "Sálvia")
	if before.GetSpeedFt() != 30 || before.GetSize() != rulesv1.CreatureSize_CREATURE_SIZE_MEDIUM {
		t.Fatalf("Sálvia as herself = %d ft, %v", before.GetSpeedFt(), before.GetSize())
	}

	// Who may: not another player (permission_denied); a beast the level does not
	// allow, or that is not a beast, is refused with the typed reason.
	_, err := s.assume(t, s.caio, s.bri, wolfKey)
	wantCode(t, "AssumeWildShape by another player", err, connect.CodePermissionDenied)
	for _, key := range []string{"monster:bat", "monster:brown-bear", "monster:goblin", "monster:nothing"} {
		_, err = s.assume(t, s.bia, s.bri, key)
		wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WILD_SHAPE_BEAST_NOT_ALLOWED)
	}
	if got := used(s.vitals(t, s.bri), wildShapeResource); got != 0 {
		t.Errorf("uses spent after the refusals = %d, want none", got)
	}
	// It needs her turn; and a wizard has no Wild Shape.
	_, err = s.assume(t, s.ana, s.pens, wolfKey)
	wantEncounterBlocked(t, err, blockedNotYourTurn)

	res := s.mustAssume(t, s.bia, s.bri, wolfKey)
	w := res.GetVitals().GetWildShape()
	if w.GetBeastKey() != wolfKey || w.GetBeastNamePt() != "Lobo" || w.GetHitPointsCurrent() != 11 || w.GetHitPointsMax() != 11 {
		t.Fatalf("the form = %v, want a wolf at 11/11", w)
	}
	if res.GetVitals().GetHitPointsCurrent() != 38 || used(res.GetVitals(), wildShapeResource) != 1 {
		t.Errorf("vitals = %v, want her own 38 PV waiting and one use spent", res.GetVitals())
	}
	wolf := byLabel(t, res.GetEncounter(), "Sálvia")
	if !wolf.GetActionUsed() || wolf.GetBonusActionUsed() {
		t.Errorf("economy = action %v, bonus %v, want only the action spent", wolf.GetActionUsed(), wolf.GetBonusActionUsed())
	}
	if wolf.GetSpeedFt() != 40 || wolf.GetSize() != rulesv1.CreatureSize_CREATURE_SIZE_MEDIUM || wolf.GetWildShapeBeastKey() != wolfKey {
		t.Errorf("combatant = %d ft, %v, form %q; want the wolf's 40 ft (12 m), Medium", wolf.GetSpeedFt(), wolf.GetSize(), wolf.GetWildShapeBeastKey())
	}
	// The master reads the wolf's armor class, and every number is the beast's.
	if got := byLabel(t, s.get(t, s.master), "Sálvia").GetArmorClass(); got != 13 {
		t.Errorf("armor class (master) = %d, want the wolf's 13", got)
	}
	opts := s.mustOptions(t, s.bia, res.GetEncounter(), "Sálvia")
	if attackOption(opts, wolfBite) == nil || len(opts.GetOptions().GetSpells()) != 0 {
		t.Errorf("turn options = attacks %v, spells %v; want the wolf's Bite and no spells", opts.GetOptions().GetAttacks(), opts.GetOptions().GetSpells())
	}
	if got := opts.GetOptions().GetEconomy().GetMovement().GetSpeedFt(); got != 40 {
		t.Errorf("movement = %d ft, want 40", got)
	}

	// RN-20: everyone who sees her sees the wolf; only the master and her player get
	// the beast's hit points.
	for name, tc := range map[string]struct {
		u       *user
		numbers bool
	}{"master": {s.master, true}, "owner": {s.bia, true}, "another player": {s.caio, false}, "a third player": {s.ana, false}} {
		got := byLabel(t, s.get(t, tc.u), "Sálvia")
		if got.GetWildShapeBeastKey() != wolfKey || got.GetWildShapeBeastNamePt() != "Lobo" {
			t.Errorf("%s sees form %q, want the wolf", name, got.GetWildShapeBeastKey())
		}
		if (got.WildShapeHitPointsCurrent != nil) != tc.numbers || (got.WildShapeHitPointsMax != nil) != tc.numbers {
			t.Errorf("%s: beast hit points = %v/%v, want numbers: %v", name, got.WildShapeHitPointsCurrent, got.WildShapeHitPointsMax, tc.numbers)
		}
		if tc.numbers && got.GetWildShapeHitPointsCurrent() != 11 {
			t.Errorf("%s: beast hit points = %d, want 11", name, got.GetWildShapeHitPointsCurrent())
		}
	}

	// No spells in the form, in a combat or out of it, with a typed reason.
	_, err = s.cast(t, s.bia, res.GetEncounter(), "Sálvia", conjureAnimals, slotOfLevel(3), nil, func(r *playv1.CastSpellRequest) {
		r.Roll = &playv1.CastSpellRequest_D20Face{D20Face: 10}
		r.Summon = &playv1.SummonChoice{Option: 1, CreatureKeys: []string{direWolf, direWolf}}
	})
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WILD_SHAPE_NO_SPELLS)
	_, err = s.castSummon(t, s.bia, s.bri, conjureAnimals, slotOfLevel(3), 1, []string{direWolf, direWolf})
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WILD_SHAPE_NO_SPELLS)
	if got := usedSlots(s.vitals(t, s.bri), 3); got != 0 {
		t.Errorf("3rd-circle slots spent = %d, want none", got)
	}

	// Already a wolf, and the action is used: neither a second form nor a second use.
	_, err = s.assume(t, s.bia, s.bri, "monster:wolf")
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ALREADY_IN_WILD_SHAPE)

	// Next round the wolf bites: its attack and its damage dice are the beast's
	// (2d4 + 2), and Irmã's turn passed with the form still on.
	a := s.armed
	next := a.passTo(t, a.mustEndTurn(t, a.bia, res.GetEncounter()), "Sálvia")
	if got := byLabel(t, next, "Sálvia"); got.GetActionUsed() || got.GetWildShapeBeastKey() != wolfKey {
		t.Fatalf("Sálvia in the next round = action used %v, form %q, want her turn again and the wolf", got.GetActionUsed(), got.GetWildShapeBeastKey())
	}
	hit := a.mustAttack(t, a.bia, next, "Sálvia", wolfBite, "Capitão Goblin", d20(19))
	if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT {
		t.Fatalf("the wolf's bite = %v, want a hit", hit.GetRoll())
	}
	if p := hit.GetPendingDamage(); p.GetDiceCount() != 2 || p.GetDiceSides() != 4 || p.GetBonus() != 2 {
		t.Errorf("bite damage = %v, want 2d4 + 2", p)
	}

	// "Voltar à forma normal" is a bonus action: it needs the turn, and the second
	// leave finds her in her own shape.
	left := s.mustLeave(t, s.bia, s.bri)
	if left.GetVitals().GetWildShape() != nil || left.GetVitals().GetHitPointsCurrent() != 38 {
		t.Errorf("after leaving = %v, want her own shape and 38 PV", left.GetVitals())
	}
	herself := byLabel(t, left.GetEncounter(), "Sálvia")
	if herself.GetSpeedFt() != 30 || herself.GetWildShapeBeastKey() != "" || !herself.GetBonusActionUsed() {
		t.Errorf("combatant after leaving = %d ft, form %q, bonus used %v; want 30 ft, her shape, the bonus action spent", herself.GetSpeedFt(), herself.GetWildShapeBeastKey(), herself.GetBonusActionUsed())
	}
	_, err = s.leave(t, s.bia, s.bri)
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_IN_WILD_SHAPE)
	// Her action is used: a second Wild Shape this turn is refused; the master may.
	_, err = s.assume(t, s.bia, s.bri, wolfKey)
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED)
	s.mustAssume(t, s.master, s.bri, wolfKey)

	// The events keep ids and keys only, never a name.
	started := s.lastPayload(t, eventWildShapeStarted)
	if started["beast"] != wolfKey || started["character_id"] != s.bri.GetId() {
		t.Errorf("wild_shape_started = %v", started)
	}
	if ended := s.lastPayload(t, eventWildShapeEnded); ended["reason"] != endedByLeaving {
		t.Errorf("wild_shape_ended = %v, want the reason %q", ended, endedByLeaving)
	}
}

// TestMR037_WildShapeDamageGoesToTheBeast: damage hits the beast's pool; healing in
// the form heals the beast; at 0 the form ends and the damage left over carries to
// the druid (SRD); the master's undo puts the form and the numbers back.
func TestMR037_WildShapeDamageGoesToTheBeast(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	a := s.armed
	s.fight(t)
	e := s.mustAssume(t, s.bia, s.bri, wolfKey).GetEncounter()

	beast := func() *playv1.WildShapeState { return s.vitals(t, s.bri).GetWildShape() }
	// The Capitão hits (a reaction, a 15 + 4 = 19 against the wolf's 13) for 6 + 2.
	hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Sálvia", func(r *playv1.RollAttackRequest) {
		r.Roll = &playv1.RollAttackRequest_D20Face{D20Face: 15}
		r.AsReaction = true
	})
	if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT {
		t.Fatalf("the Capitão's attack = %v, want a hit against the wolf's armor class", hit.GetRoll())
	}
	dmg := a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), typedDamage(6)).GetPendingDamage()
	if dmg.GetAmount() != 8 {
		t.Fatalf("damage = %v, want 8", dmg)
	}
	if _, err := a.settle(t, a.master, e, dmg.GetId(), true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	if w := beast(); w.GetHitPointsCurrent() != 3 || a.vitals(t, s.bri).GetHitPointsCurrent() != 38 {
		t.Fatalf("after 8 damage: beast %v, Sálvia %d PV; want the beast at 3/11 and her own 38 untouched", w, a.vitals(t, s.bri).GetHitPointsCurrent())
	}

	// Irmã heals her wolf with Curar Ferimentos (1d8 = 4, + 3): the beast, to 10.
	a.passTo(t, e, "Irmã")
	cast := a.mustCast(t, s.dani, a.get(t, s.dani), "Irmã", cureWounds, slotOfLevel(1), a.at(t, "Sálvia"), noCastRoll)
	a.h.roller.queue(4)
	healed := a.mustDamage(t, s.dani, a.get(t, s.dani), cast.GetCast().GetPendingDamages()[0].GetId(), inAppDamage).GetPendingDamage()
	if healed.GetAmount() != 7 {
		t.Fatalf("the heal = %v, want 7", healed)
	}
	if w := beast(); w.GetHitPointsCurrent() != 10 || a.vitals(t, s.bri).GetHitPointsCurrent() != 38 {
		t.Errorf("after the heal: beast %v, Sálvia %d PV; want the beast at 10/11 and her own 38", w, a.vitals(t, s.bri).GetHitPointsCurrent())
	}

	// The goblin's critical hit: 2d6 typed 12 + 2 = 14 against 10: the beast falls and 4
	// carries to the druid.
	e = a.get(t, a.master)
	crit := a.mustAttack(t, a.master, e, "Goblin", sword, "Sálvia", func(r *playv1.RollAttackRequest) {
		r.Roll = &playv1.RollAttackRequest_D20Face{D20Face: 20}
		r.AsReaction = true
	})
	big := a.mustDamage(t, a.master, e, crit.GetPendingDamage().GetId(), typedDamage(12)).GetPendingDamage()
	if big.GetAmount() != 14 {
		t.Fatalf("critical damage = %v, want 14", big)
	}
	if _, err := a.settle(t, a.master, e, big.GetId(), true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	after := a.vitals(t, s.bri)
	if after.GetWildShape() != nil || after.GetHitPointsCurrent() != 34 {
		t.Fatalf("after the beast fell: %v, want her own shape and 38 - 4 = 34 PV", after)
	}
	if got := byLabel(t, a.get(t, a.master), "Sálvia"); got.GetSpeedFt() != 30 || got.GetWildShapeBeastKey() != "" {
		t.Errorf("combatant after the beast fell = %d ft, form %q, want her own numbers", got.GetSpeedFt(), got.GetWildShapeBeastKey())
	}
	ended := a.lastPayload(t, eventWildShapeEnded)
	if ended["reason"] != endedByDamage || ended["beast"] != wolfKey || ended["carried_damage"] != float64(4) {
		t.Errorf("wild_shape_ended = %v, want the beast, the reason %q and 4 carried", ended, endedByDamage)
	}

	// The master's undo of the damage: the wolf is back at 10, her own 38 too.
	a.undoLast(t, a.get(t, a.master))
	if w, hp := beast(), a.vitals(t, s.bri).GetHitPointsCurrent(); w.GetHitPointsCurrent() != 10 || w.GetBeastKey() != wolfKey || hp != 38 {
		t.Errorf("after the undo: beast %v, Sálvia %d PV; want the wolf at 10 and 38 PV", w, hp)
	}
	if got := byLabel(t, a.get(t, a.master), "Sálvia"); got.GetSpeedFt() != 40 || got.GetWildShapeBeastKey() != wolfKey {
		t.Errorf("combatant after the undo = %d ft, form %q, want the wolf's numbers back", got.GetSpeedFt(), got.GetWildShapeBeastKey())
	}
}

// TestMR037_WildShapeExactDamageEndsTheFormWithNothingLeftOver: a damage equal to the
// beast's hit points ends the form and carries nothing over.
func TestMR037_WildShapeExactDamageEndsTheFormWithNothingLeftOver(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	a := s.armed
	s.fight(t)
	e := s.mustAssume(t, s.bia, s.bri, wolfKey).GetEncounter()
	s.correct(t, s.bri, func(r *playv1.AdjustCharacterVitalsRequest) { r.WildShapeHitPointsCurrent = new(int32(8)) })
	hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Sálvia", func(r *playv1.RollAttackRequest) {
		r.Roll = &playv1.RollAttackRequest_D20Face{D20Face: 15}
		r.AsReaction = true
	})
	dmg := a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), typedDamage(6)).GetPendingDamage() // 6 + 2 = 8
	if _, err := a.settle(t, a.master, e, dmg.GetId(), true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	after := a.vitals(t, s.bri)
	if after.GetWildShape() != nil || after.GetHitPointsCurrent() != 38 {
		t.Errorf("after 8 damage on a beast with 8: %v, want her own shape and all 38 PV", after)
	}
	if ended := a.lastPayload(t, eventWildShapeEnded); ended["carried_damage"] != nil {
		t.Errorf("wild_shape_ended = %v, want nothing carried", ended)
	}
}

// TestMR037_WildShapeUndoOfTheActions: the master's undo of the action that started
// the form ends it and gives the use, the action and the speed back; the undo of
// "Voltar à forma normal" puts the wolf back with its hit points.
func TestMR037_WildShapeUndoOfTheActions(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	a := s.armed
	s.fight(t)

	// Before: the economy and the use.
	s.mustAssume(t, s.bia, s.bri, wolfKey)
	a.undoLast(t, a.get(t, a.master))
	own := a.vitals(t, s.bri)
	got := byLabel(t, a.get(t, a.master), "Sálvia")
	if own.GetWildShape() != nil || used(own, wildShapeResource) != 0 || got.GetActionUsed() || got.GetSpeedFt() != 30 || got.GetWildShapeBeastKey() != "" {
		t.Errorf("after undoing the start: vitals %v, combatant action used %v, %d ft, form %q; want nothing changed", own, got.GetActionUsed(), got.GetSpeedFt(), got.GetWildShapeBeastKey())
	}

	// Start again, hurt the wolf (the master's correction), leave, and undo the leaving.
	s.mustAssume(t, s.bia, s.bri, wolfKey)
	s.correct(t, s.bri, func(r *playv1.AdjustCharacterVitalsRequest) { r.WildShapeHitPointsCurrent = new(int32(6)) })
	s.mustLeave(t, s.bia, s.bri)
	if w := a.vitals(t, s.bri).GetWildShape(); w != nil {
		t.Fatalf("form after leaving = %v, want none", w)
	}
	// The correction is an event with no combat, so the undo of the leaving is the
	// last action: the wolf comes back with the hit points it had.
	a.undoLast(t, a.get(t, a.master))
	w := a.vitals(t, s.bri).GetWildShape()
	got = byLabel(t, a.get(t, a.master), "Sálvia")
	if w.GetBeastKey() != wolfKey || w.GetHitPointsCurrent() != 6 || got.GetBonusActionUsed() || got.GetSpeedFt() != 40 {
		t.Errorf("after undoing the leaving: form %v, bonus used %v, %d ft; want the wolf at 6 and the bonus action back", w, got.GetBonusActionUsed(), got.GetSpeedFt())
	}
}

// TestMR037_WildShapeOutsideACombat: the form is taken in a session without a combat
// at no action cost, spends a use (2 per short rest), blocks spells, and lasts until
// it is ended: by the beast falling to 0 (the master's correction), by leaving, or
// by the master. The master may do it for any druid; the events carry ids only.
func TestMR037_WildShapeOutsideACombat(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	a := s.armed

	// Another player's character: permission_denied; the master and the owner may.
	_, err := s.assume(t, s.caio, s.bri, wolfKey)
	wantCode(t, "AssumeWildShape by another player", err, connect.CodePermissionDenied)
	key := newKey()
	assumeWith := func() (*playv1.AssumeWildShapeResponse, error) {
		res, err := s.bia.play.AssumeWildShape(t.Context(), connect.NewRequest(&playv1.AssumeWildShapeRequest{
			CampaignId: s.campaignID, CharacterId: s.bri.GetId(), BeastKey: wolfKey, IdempotencyKey: key,
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	res, err := assumeWith()
	if err != nil || res.GetEncounter() != nil || res.GetVitals().GetWildShape().GetBeastKey() != wolfKey {
		t.Fatalf("AssumeWildShape() = %v, %v, want the form and no combat", res, err)
	}
	// A retry with the same key changes nothing: one use, one event.
	if again, err := assumeWith(); err != nil || again.GetVitals().GetWildShape().GetBeastKey() != wolfKey || used(again.GetVitals(), wildShapeResource) != 1 {
		t.Errorf("a retry = %v, %v, want the same form and one use", again, err)
	}
	if n := a.eventCount(t, eventWildShapeStarted); n != 1 {
		t.Errorf("wild_shape_started events = %d, want 1", n)
	}
	if _, err := s.castSummon(t, s.bia, s.bri, conjureAnimals, slotOfLevel(3), 1, []string{direWolf, direWolf}); err == nil {
		t.Error("CastSummon in the form succeeded, want WILD_SHAPE_NO_SPELLS")
	} else {
		wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WILD_SHAPE_NO_SPELLS)
	}
	// It lasts across sessions' calls: a read of the live session has it.
	var seen bool
	for _, v := range s.bia.liveSession(t, s.campaignID).GetVitals() {
		seen = seen || v.GetWildShape().GetBeastKey() == wolfKey
	}
	if !seen {
		t.Error("the player's live session does not show the form")
	}

	// Two uses per short rest: leave, become again, leave, and the third is refused.
	s.mustLeave(t, s.bia, s.bri)
	s.mustAssume(t, s.master, s.bri, "monster:mastiff")
	s.mustLeave(t, s.master, s.bri)
	if got := used(a.vitals(t, s.bri), wildShapeResource); got != 2 {
		t.Fatalf("uses spent = %d, want 2", got)
	}
	_, err = s.assume(t, s.bia, s.bri, wolfKey)
	wantEncounterBlocked(t, err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_USES)
	if a.vitals(t, s.bri).GetWildShape() != nil {
		t.Error("a refused Wild Shape left a form behind")
	}
	// The master's correction takes the beast to 0: the form ends, the druid's own
	// hit points stay. With no form, a correction of the beast is refused.
	s.correct(t, s.bri, func(r *playv1.AdjustCharacterVitalsRequest) { r.HitPointsCurrent = new(int32(31)) })
	_, err = s.master.adjust(t, s.campaignID, s.bri.GetId(), func(r *playv1.AdjustCharacterVitalsRequest) { r.WildShapeHitPointsCurrent = new(int32(3)) })
	wantCode(t, "AdjustCharacterVitals of a beast pool with no form", err, connect.CodeInvalidArgument)
	if _, err := s.master.play.AdjustCharacterVitals(t.Context(), connect.NewRequest(&playv1.AdjustCharacterVitalsRequest{
		CampaignId: s.campaignID, CharacterId: s.bri.GetId(), IdempotencyKey: newKey(), ResourcesUsed: []*playv1.ResourceUsed{{Key: wildShapeResource, Used: 0}},
	})); err != nil {
		t.Fatalf("giving the uses back: %v", err)
	}
	s.mustAssume(t, s.bia, s.bri, wolfKey)
	v := s.correct(t, s.bri, func(r *playv1.AdjustCharacterVitalsRequest) { r.WildShapeHitPointsCurrent = new(int32(4)) })
	if v.GetWildShape().GetHitPointsCurrent() != 4 {
		t.Errorf("beast after the correction = %v, want 4", v.GetWildShape())
	}
	_, err = s.master.adjust(t, s.campaignID, s.bri.GetId(), func(r *playv1.AdjustCharacterVitalsRequest) { r.WildShapeHitPointsCurrent = new(int32(12)) })
	wantCode(t, "a beast pool above the stat block's 11", err, connect.CodeInvalidArgument)
	v = s.correct(t, s.bri, func(r *playv1.AdjustCharacterVitalsRequest) { r.WildShapeHitPointsCurrent = new(int32(0)) })
	if v.GetWildShape() != nil || v.GetHitPointsCurrent() != 31 {
		t.Errorf("after the beast went to 0: %v, want her own shape and 31 PV", v)
	}
	if ended := a.lastPayload(t, eventWildShapeEnded); ended["reason"] != endedByMaster {
		t.Errorf("wild_shape_ended = %v, want the reason %q", ended, endedByMaster)
	}
}

// TestMR037_WildShapeLeavesOtherCombatantsAlone: the form is the druid's alone; no
// other combatant carries one.
func TestMR037_WildShapeLeavesOtherCombatantsAlone(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	s.fight(t)
	s.mustAssume(t, s.bia, s.bri, wolfKey)
	for _, label := range []string{"Toren", "Pensantus", "Irmã", "Goblin"} {
		if c := byLabel(t, s.get(t, s.master), label); c.GetWildShapeBeastKey() != "" || c.WildShapeHitPointsCurrent != nil || c.GetFamiliarSightCreatureId() != "" {
			t.Errorf("%s carries a form: %v", label, c)
		}
	}
}

// A trap damage applied outside a combat falls on a druid's beast pool first, and what is left
// goes to the druid when the beast falls (RN-02), as it does in a combat.
func TestOutOfCombatTrapDamageGoesToTheBeastForm(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		dice         [2]int
		wantBeast    int32 // beast HP after, 0 = form ended
		wantDruidLos int32
	}{
		{"beast absorbs", [2]int{3, 4}, 4, 0},          // 7 of 11
		{"overflow ends the form", [2]int{6, 6}, 0, 1}, // 12 vs 11: 1 left over
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newShapers(t)
			content, err := testRules()
			if err != nil {
				t.Fatal(err)
			}
			msvc, err := maps.New(maps.Config{Pool: s.h.pool, Characters: s.h.chars, Live: s.h.svc, Rules: content, Combats: s.h.svc, Logger: slog.New(slog.DiscardHandler)})
			if err != nil {
				t.Fatal(err)
			}
			s.h.svc.SetTraps(msvc)
			msvc.SetTrapFirer(s.h.svc)
			srv := httpserver.New(httpserver.Config{Logger: slog.New(slog.DiscardHandler)})
			msvc.Mount(srv.Handle, mapsSessions{testSessions}, s.h.camps, connect.WithRequireConnectProtocolHeader())
			server := httptest.NewServer(srv.Handler())
			t.Cleanup(server.Close)
			mc := mapsv1connect.NewMapServiceClient(&http.Client{Transport: userTransport{userID: s.master.id, next: server.Client().Transport}}, server.URL)

			mapID := s.h.newMapOf(s.campaignID, 24, 1200, 800)
			if _, err := s.master.play.SetCurrentMap(t.Context(), connect.NewRequest(&playv1.SetCurrentMapRequest{CampaignId: s.campaignID, MapId: mapID})); err != nil {
				t.Fatal(err)
			}
			x, y := sq(5, 5)
			spec := &mapsv1.TrapSpec{
				NoticeDc: 12, FindDc: 15, AreaSize: 1, Trigger: rulesv1.TrapTrigger_TRAP_TRIGGER_MANUAL,
				Effect: &rulesv1.TrapEffect{Damage: []*rulesv1.TrapDamage{{Dice: "2d6", DamageTypeKey: "damage-type:bludgeoning"}}},
			}
			pt, err := mc.CreateMapPoint(t.Context(), connect.NewRequest(&mapsv1.CreateMapPointRequest{
				CampaignId: s.campaignID, MapId: mapID, Kind: mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, Name: "Fosso", XBp: x, YBp: y, Trap: spec,
			}))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := mc.PlaceMapToken(t.Context(), connect.NewRequest(&mapsv1.PlaceMapTokenRequest{CampaignId: s.campaignID, MapId: mapID, CharacterId: s.bri.GetId(), XBp: x, YBp: y})); err != nil {
				t.Fatal(err)
			}
			s.mustAssume(t, s.bia, s.bri, wolfKey)
			before := s.vitals(t, s.bri)
			if before.GetWildShape() == nil {
				t.Fatal("no beast form")
			}
			beastBefore := before.GetWildShape().GetHitPointsCurrent()
			s.h.roller.queue(tc.dice[0], tc.dice[1])
			if _, err := s.master.play.FireTrap(t.Context(), connect.NewRequest(&playv1.FireTrapRequest{CampaignId: s.campaignID, MapId: mapID, PointId: pt.Msg.GetPoint().GetId(), IdempotencyKey: newKey()})); err != nil {
				t.Fatal(err)
			}
			list, err := s.master.play.ListTrapDamages(t.Context(), connect.NewRequest(&playv1.ListTrapDamagesRequest{CampaignId: s.campaignID}))
			if err != nil || len(list.Msg.GetDamages()) != 1 {
				t.Fatalf("damages = %v %v", list, err)
			}
			if _, err := s.master.play.ApplyTrapDamage(t.Context(), connect.NewRequest(&playv1.ApplyTrapDamageRequest{CampaignId: s.campaignID, TrapDamageId: list.Msg.GetDamages()[0].GetId(), IdempotencyKey: newKey()})); err != nil {
				t.Fatal(err)
			}
			after := s.vitals(t, s.bri)
			beastAfter := int32(0)
			if w := after.GetWildShape(); w != nil {
				beastAfter = w.GetHitPointsCurrent()
			}
			t.Logf("druid hp %d -> %d; beast %d -> %d (form kept: %v)", before.GetHitPointsCurrent(), after.GetHitPointsCurrent(), beastBefore, beastAfter, after.GetWildShape() != nil)
			if beastAfter != tc.wantBeast {
				t.Errorf("beast hp = %d, want %d: the trap damage bypassed the beast's pool", beastAfter, tc.wantBeast)
			}
			if got := before.GetHitPointsCurrent() - after.GetHitPointsCurrent(); got != tc.wantDruidLos {
				t.Errorf("druid lost %d own hp, want %d", got, tc.wantDruidLos)
			}
		})
	}
}

// sessionEventCount counts the events of a kind written into one game session.
func sessionEventCount(t *testing.T, h *harness, sessionID, kind string) int {
	t.Helper()
	var n int
	if err := h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events WHERE game_session_id = $1 AND kind = $2`, sessionID, kind).Scan(&n); err != nil {
		t.Fatalf("count %s events: %v", kind, err)
	}
	return n
}

// A trap damage left from an ended session is settled in the open one: a beast form that falls under
// it ends there, in the log and in the combat, not in the session where the trap fired.
func TestTrapDamageOfAnEndedSessionEndsTheFormInTheOpenSession(t *testing.T) {
	t.Parallel()
	s := newShapers(t)
	ctx := t.Context()
	s1 := s.master.liveSession(t, s.campaignID).GetGameSession()
	d, err := s.h.svc.queries.InsertTrapDamage(ctx, playdb.InsertTrapDamageParams{
		GameSessionID: s1.GetId(), TrapPointID: uuid.New().String(), FireID: uuid.New().String(), CharacterID: s.bri.GetId(),
		DiceCount: 0, DiceSides: 0, DiceBonus: 50, DamageType: "damage-type:piercing", Faces: []int32{}, RollTotal: 50, Amount: 50, CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("InsertTrapDamage() error = %v", err)
	}
	s.master.end(t, s1)
	s2 := s.master.start(t, s.campaignID).GetGameSession()
	if s2.GetId() == s1.GetId() {
		t.Fatalf("session 2 has session 1's ID")
	}
	s.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: s.goblin.GetId()}},
		npcRolls: []int{1},
		players:  map[string]int32{"Sálvia": 20, "Irmã": 15, "Pensantus": 12, "Toren": 10},
		reveal:   []string{"Goblin"},
		theatre:  true,
	})
	s.mustAssume(t, s.bia, s.bri, wolfKey)
	if got := byLabel(t, s.get(t, s.master), "Sálvia"); got.GetSpeedFt() != 40 {
		t.Fatalf("Sálvia as a wolf = %d ft, want 40", got.GetSpeedFt())
	}
	if _, err := s.master.play.ApplyTrapDamage(ctx, connect.NewRequest(&playv1.ApplyTrapDamageRequest{CampaignId: s.campaignID, TrapDamageId: d.ID, IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("ApplyTrapDamage() error = %v", err)
	}
	if v := s.vitals(t, s.bri); v.GetWildShape() != nil {
		t.Fatalf("the beast is still on after 50 damage: %v", v)
	}
	if got := sessionEventCount(t, s.h, s2.GetId(), eventWildShapeEnded); got != 1 {
		t.Errorf("wild_shape_ended events in the open session 2 = %d, want 1 (in session 1: %d)", got, sessionEventCount(t, s.h, s1.GetId(), eventWildShapeEnded))
	}
	if got := byLabel(t, s.get(t, s.master), "Sálvia"); got.GetSpeedFt() != 30 || got.GetWildShapeBeastKey() != "" {
		t.Errorf("combatant in session 2's combat after the beast fell = %d ft, form %q, want her own 30 ft and no form", got.GetSpeedFt(), got.GetWildShapeBeastKey())
	}
}
