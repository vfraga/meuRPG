package play

import (
	"strings"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The reaction window (PM-04): an action that a reaction can change waits for the
// reactor, and the master, who answers for the NPCs, is the only one who knows whether
// there is one. These tests run the SRD's Mage, which has Shield and Counterspell on
// its Spellcasting trait (3 slots of the 3rd level, 4 of the 1st).

const mageKey = "monster:mage"

// mageFight is a combat of the party and a Mage ("Mago"), revealed, next to Toren
// (who is first), with the goblin the setup brings out of the way.
func mageFight(t *testing.T) (*armed, *playv1.Encounter) {
	t.Helper()
	return monsterFight(t, mageKey, "Mago")
}

// monsterFight is the fight with one stat block NPC of the key, labeled label, next to Toren.
func monsterFight(t *testing.T, key, label string) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newCasters(t)
	e := a.monsterSetup(t, false)
	a.h.roller.queue(1)
	a.mustAddMonsters(t, e, func(r *playv1.AddMonstersRequest) { r.CreatureKey, r.Count = key, 1 })
	e = a.get(t, a.master)
	for label, sq := range map[string][2]int32{"Toren": {6, 5}, "Pensantus": {5, 5}, "Brisa": {5, 6}, "Goblin": {12, 8}, label: {7, 5}} {
		if _, err := a.master.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, label), IdempotencyKey: newKey(), Col: sq[0], Row: sq[1],
		})); err != nil {
			t.Fatalf("MoveCombatant(%s) error = %v", label, err)
		}
	}
	for _, shown := range []string{label, "Goblin"} {
		if _, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, shown), IdempotencyKey: newKey(), Hidden: false,
		})); err != nil {
			t.Fatalf("SetCombatantHidden(%s) error = %v", shown, err)
		}
	}
	e = a.begin(t, e)
	return a, e
}

// TestAnNPCsShieldHoldsAPlayersAttack: Toren hits the Mage, which has Shield and a slot
// for it: the turn waits for the master, who alone knows why (the player reads "Esperando
// o mestre" and nothing else, RN-10); the master sees what Shield would change and answers.
func TestAnNPCsShieldHoldsAPlayersAttack(t *testing.T) {
	t.Parallel()
	a, e := mageFight(t)
	if cur := a.get(t, a.master).GetCurrentCombatantId(); cur != a.id(t, "Toren") {
		t.Fatalf("the turn is %s's, want Toren's", cur)
	}
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Mago", d20(9)) // 9 + 6 = 15 against the Mage's AC 12
	if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || hit.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_REACTION {
		t.Fatalf("Toren's attack = %v / %v, want a hit that waits for the reaction", hit.GetRoll(), hit.GetPendingDamage())
	}

	// What the players read: the wait, and nothing of the reactor.
	for who, u := range map[string]*user{"Toren's player": a.caio, "Pensantus's player": a.ana} {
		got := a.get(t, u)
		if len(got.GetReactionWindows()) != 0 || len(got.GetReactionPrompts()) != 0 {
			t.Errorf("%s got windows %v / prompts %v: the reactor is an NPC", who, got.GetReactionWindows(), got.GetReactionPrompts())
		}
		if title := got.GetReactionWait().GetTitlePt(); title != "Esperando o mestre" {
			t.Errorf("%s reads %q, want \"Esperando o mestre\"", who, title)
		}
	}
	if d := a.get(t, a.caio).GetReactionWait().GetDetailPt(); d != "O resultado do seu ataque sai quando ele responder." {
		t.Errorf("the attacker's detail = %q", d)
	}
	if d := a.get(t, a.ana).GetReactionWait().GetDetailPt(); d != "O turno continua quando ele responder." {
		t.Errorf("another player's detail = %q", d)
	}

	// The master sees the window, with the numbers that are his.
	w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_SHIELD)
	if w == nil || !w.GetForYou() || !w.GetAnswerNow() || w.GetReactorLabel() != "Mago" || w.GetReactorIsPlayer() {
		t.Fatalf("the master's window = %v, want Mago's Shield to answer now", w)
	}
	tr := w.GetTrigger()
	if tr.GetAttackTotal() != 15 || tr.GetTargetArmorClass() != 12 || tr.GetArmorClassWithShield() != 17 || tr.GetActorLabel() != "Toren" {
		t.Errorf("the master's trigger = %v, want total 15 against armor class 12 (17 with Shield)", tr)
	}
	if len(w.GetShield().GetSlots()) != 5 {
		t.Errorf("the Mage's Shield slots = %v, want the five levels of its stat block", w.GetShield().GetSlots())
	}
	for _, sl := range w.GetShield().GetSlots() {
		if sl.GetFree() < 1 || sl.GetTotal() < sl.GetFree() {
			t.Errorf("the Mage's Shield slot %v, want free >= 1 and a total that holds it", sl)
		}
	}

	// The turn waits: no damage, no second attack, no end of turn, for the player.
	_, err := a.damage(t, a.caio, e, hit.GetPendingDamage().GetId(), inAppDamage)
	wantBlockedBy(t, "RollDamage while the Mage may react", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_PENDING)
	_, err = a.attack(t, a.caio, e, "Toren", battleaxe, "Mago", d20(9))
	wantBlockedBy(t, "a second attack", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_PENDING)
	_, err = a.endTurn(t, a.caio, e, false)
	wantBlockedBy(t, "EndTurn", err, blockedPendingDamage)

	// A player cannot answer it, and the refusal is the same for a window that never existed.
	_, errReal := a.answerReaction(t, a.caio, e, w.GetId(), passAnswer)
	_, errFake := a.answerReaction(t, a.caio, e, "00000000-0000-4000-8000-000000000001", passAnswer)
	wantCode(t, "Toren's player answering the Mage's window", errReal, connect.CodePermissionDenied)
	wantCode(t, "Toren's player answering a window that does not exist", errFake, connect.CodePermissionDenied)
	if errReal.Error() != errFake.Error() {
		t.Errorf("a real window is refused with %q, an invented one with %q: they must read the same (RN-10)", errReal, errFake)
	}

	// The master uses Shield for the Mage: 15 against 17 is a miss.
	res := a.mustAnswer(t, a.master, e, w.GetId(), useAnswer(1))
	if !res.GetResult().GetShield().GetStopped() || !res.GetResult().GetUsed() {
		t.Errorf("the answer's result = %v, want Shield used and the attack stopped", res.GetResult())
	}
	mage := byLabel(t, res.GetEncounter(), "Mago")
	if !mage.GetReactionUsed() || mage.GetArmorClassBonus() != 5 {
		t.Errorf("the Mage after Shield = reaction used %v, bonus %d; want the reaction used and +5", mage.GetReactionUsed(), mage.GetArmorClassBonus())
	}
	if n := len(a.get(t, a.caio).GetReactionWindows()) + len(a.get(t, a.master).GetReactionWindows()); n != 0 || a.get(t, a.caio).GetReactionWait() != nil {
		t.Errorf("windows left after the answer: %d, wait %v", n, a.get(t, a.caio).GetReactionWait())
	}
	// The hit is gone and the attacker's turn goes on.
	if _, err := a.damage(t, a.caio, e, hit.GetPendingDamage().GetId(), inAppDamage); err == nil {
		t.Error("the stopped hit's damage was rolled")
	}
	if _, err := a.endTurn(t, a.caio, e, false); err != nil {
		t.Errorf("EndTurn() after the answer error = %v", err)
	}
	// What the Mage spent is counted on its stat block: one slot of the 1st level.
	var used string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT slots_used::TEXT FROM combatants WHERE id = $1`, mage.GetId()).Scan(&used); err != nil || !strings.Contains(used, `"1": 1`) {
		t.Errorf("the Mage's slots used = %q, %v; want one 1st-level slot", used, err)
	}
}

// pensantusTurn passes Toren's turn so that Pensantus acts.
func (a *armed) pensantusTurn(t *testing.T, e *playv1.Encounter) {
	t.Helper()
	a.passTo(t, e, "Pensantus")
}

// castMissile has Pensantus cast Magic Missile at the Goblin with a 1st-level slot.
func (a *armed) castMissile(t *testing.T, e *playv1.Encounter) (*playv1.CastSpellResponse, error) {
	t.Helper()
	return a.cast(t, a.ana, e, "Pensantus", magicMissileSpell, slotOfLevel(1), []*playv1.SpellTarget{darts(a, t, "Goblin", 3)}, noCastRoll)
}

// TestACounterspellHoldsAPlayersCastAndSpendsItsSlot: the Mage sees Pensantus cast, has
// Counterspell and a 3rd-level slot: the cast waits, nothing is spent and nothing
// happens, the player reads only "Esperando o mestre"; when the master counters it, the
// slot and the action are spent all the same and the spell has no effect (SRD, Counterspell).
func TestACounterspellHoldsAPlayersCastAndSpendsItsSlot(t *testing.T) {
	t.Parallel()
	a, e := mageFight(t)
	a.pensantusTurn(t, e)
	slotsBefore := a.vitals(t, a.pens).GetSpellSlots()

	held, err := a.castMissile(t, e)
	if err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	if held.GetCast() != nil {
		t.Fatalf("the held cast answers %v, want no cast yet", held.GetCast())
	}
	if got := a.vitals(t, a.pens).GetSpellSlots(); !sameSlots(got, slotsBefore) {
		t.Errorf("slots while the cast waits = %v, want %v: nothing is spent before the answer", got, slotsBefore)
	}
	if o := byLabel(t, a.get(t, a.master), "Pensantus"); o.GetActionUsed() {
		t.Error("the action is spent while the cast waits")
	}
	if got := a.get(t, a.ana); got.GetReactionWait().GetTitlePt() != "Esperando o mestre" || got.GetReactionWait().GetDetailPt() != "A sua conjuração se resolve quando ele responder." || len(got.GetReactionWindows()) != 0 {
		t.Errorf("the caster reads %v / %v, want the wait for the master only", got.GetReactionWait(), got.GetReactionWindows())
	}
	// The turn waits.
	_, err = a.endTurn(t, a.ana, e, false)
	wantBlockedBy(t, "EndTurn while the cast waits", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_PENDING)
	_, err = a.castMissile(t, e)
	wantBlockedBy(t, "a second cast", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_PENDING)

	// The master sees the spell and what each slot does; the prompt keeps the spell to itself.
	w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_COUNTERSPELL)
	if w == nil || w.GetTrigger().GetSpellKey() != magicMissileSpell || w.GetTrigger().GetSpellLevel() != 1 || len(w.GetTrigger().GetSlotEffects()) != 3 {
		t.Fatalf("the master's window = %v, want Counterspell with the spell and a slot of the 3rd, 4th and 5th level", w)
	}
	for _, se := range w.GetTrigger().GetSlotEffects() {
		if !se.GetNoCheck() {
			t.Errorf("slot %v needs a check against a 1st-level spell: %v", se.GetSlot(), se)
		}
	}
	res := a.mustAnswer(t, a.master, e, w.GetId(), useAnswer(3))
	if c := res.GetResult().GetCounterspell(); !c.GetCountered() || c.GetSpellKey() != magicMissileSpell {
		t.Fatalf("the result = %v, want the spell countered", res.GetResult())
	}
	got := a.vitals(t, a.pens).GetSpellSlots()
	if sameSlots(got, slotsBefore) {
		t.Errorf("slots after the counter = %v: the countered spell's slot is spent", got)
	}
	after := a.get(t, a.master)
	if !byLabel(t, after, "Pensantus").GetActionUsed() || !byLabel(t, after, "Mago").GetReactionUsed() {
		t.Errorf("after the counter: Pensantus' action used %v, the Mage's reaction used %v; want both", byLabel(t, after, "Pensantus").GetActionUsed(), byLabel(t, after, "Mago").GetReactionUsed())
	}
	if goblin := byLabel(t, after, "Goblin"); goblin.GetHitPointsCurrent() != goblin.GetHitPointsMax() {
		t.Errorf("the Goblin took damage from a countered spell: %v", goblin)
	}
	if got := a.get(t, a.ana); got.GetReactionWait() != nil {
		t.Errorf("the caster still waits: %v", got.GetReactionWait())
	}
	if _, err := a.endTurn(t, a.ana, e, false); err != nil {
		t.Errorf("EndTurn() after the answer error = %v", err)
	}
	// The log: the master's line has the numbers, the players' has none.
	line := func(u *user) string {
		for _, r := range a.log(t, u, e).GetRounds() {
			for _, en := range r.GetEntries() {
				if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW {
					return en.GetReactionTextPt()
				}
			}
		}
		return ""
	}
	if got := line(a.master); !strings.Contains(got, "Mago usou Contramágica (espaço de 3º nível)") || !strings.Contains(got, "anulada, sem teste") {
		t.Errorf("the master's line = %q", got)
	}
	if got := line(a.caio); !strings.Contains(got, "Mago usou Contramágica") || strings.Contains(got, "espaço") || strings.Contains(got, "nível") {
		t.Errorf("a player's line = %q, want the reactor and no numbers", got)
	}
}

// TestACounterspellPassedLetsTheCastHappen: "Deixar passar" releases the held cast,
// which happens as if nothing had waited: slot and action spent, the darts open.
func TestACounterspellPassedLetsTheCastHappen(t *testing.T) {
	t.Parallel()
	a, e := mageFight(t)
	a.pensantusTurn(t, e)
	if _, err := a.castMissile(t, e); err != nil {
		t.Fatalf("CastSpell() error = %v", err)
	}
	w := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_COUNTERSPELL)
	a.mustAnswer(t, a.master, e, w.GetId(), passAnswer)
	got := a.get(t, a.master)
	if !byLabel(t, got, "Pensantus").GetActionUsed() || byLabel(t, got, "Mago").GetReactionUsed() {
		t.Error("after the pass: the action should be spent and the Mage's reaction still its own")
	}
	// The darts opened a pending damage the caster rolls.
	var pending string
	if err := a.h.pool.QueryRow(t.Context(), `SELECT id::TEXT FROM pending_damages WHERE encounter_id = $1 AND attack_key = $2`, e.GetId(), magicMissileSpell).Scan(&pending); err != nil {
		t.Fatalf("the cast opened no pending damage: %v", err)
	}
	a.h.roller.queue(2, 2, 2)
	if _, err := a.damage(t, a.ana, e, pending, inAppDamage); err != nil {
		t.Errorf("RollDamage() after the pass error = %v", err)
	}
	if n := len(a.get(t, a.ana).GetReactionWindows()); n != 0 {
		t.Errorf("windows left: %d", n)
	}
}

func sameSlots(a, b []*playv1.SpellSlotUsage) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].GetLevel() != b[i].GetLevel() || a[i].GetUsed() != b[i].GetUsed() {
			return false
		}
	}
	return true
}

const counterspellSpell = "spell:counterspell"

// bossCaster creates an NPC with a full sheet that casts (an enemy wizard): the master
// casts for it with CastSpell, and nobody counts its slots.
func (u *user) bossCaster(t *testing.T, campaignID, name string, level int32, cantrips, known, prepared []string) string {
	t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 8, Dexterity: 14, Constitution: 12, Intelligence: 17, Wisdom: 10, Charisma: 10},
		RaceKey:    "race:human", Classes: []*charactersv1.ClassLevel{{ClassKey: "class:wizard", Level: level}},
		CantripKeys: cantrips, KnownSpellKeys: known, PreparedSpellKeys: prepared,
	}}}
	res, err := u.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_BOSS, Name: name, Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(%s) error = %v", name, err)
	}
	return res.Msg.GetCharacter().GetId()
}

// counterFight is a combat where an enemy wizard (first) fights a party whose Pensantus
// is a level 5 wizard with Counterspell.
func counterFight(t *testing.T) (*armed, *playv1.Encounter) {
	t.Helper()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 5,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.caster(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 5,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8}, nil, []string{fireBolt},
			[]string{magicMissileSpell, shieldSpell, counterspellSpell}, []string{magicMissileSpell, shieldSpell, counterspellSpell})
		a.bri = a.bia.caster(t, a.campaignID, "Brisa", "class:cleric", "race:human", 3,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 16, Charisma: 8}, []string{maceKey}, []string{sacredFlame}, nil,
			[]string{cureWounds, healingWord, guidingBolt})
	})
	id := a.master.bossCaster(t, a.campaignID, "Mago Sombrio", 7, []string{fireBolt}, []string{magicMissileSpell, "spell:fireball"}, []string{magicMissileSpell, "spell:fireball"})
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: id}}, npcRolls: []int{20}, reveal: []string{"Mago Sombrio"},
		at:      map[string][2]int32{"Mago Sombrio": {2, 5}, "Pensantus": {5, 5}, "Toren": {6, 5}, "Brisa": {5, 6}},
		players: map[string]int32{"Pensantus": 10, "Toren": 9, "Brisa": 8},
	})
	return a, e
}

// TestAPlayersCounterspellCountersAnNPCsCast: the Mago Sombrio casts Magic Missile at Toren
// and Pensantus, who sees it within 18 m, may counter it. Her prompt has no spell and no
// level (she sees that it casts, not what); the master reads "Esperando a reação de
// Pensantus"; the spell fails with her slot of the 3rd level (SRD, Counterspell).
func TestAPlayersCounterspellCountersAnNPCsCast(t *testing.T) {
	t.Parallel()
	a, e := counterFight(t)
	held, err := a.cast(t, a.master, e, "Mago Sombrio", magicMissileSpell, slotOfLevel(1), []*playv1.SpellTarget{darts(a, t, "Toren", 3)}, noCastRoll)
	if err != nil {
		t.Fatalf("CastSpell(the Mago Sombrio) error = %v", err)
	}
	if held.GetCast() != nil {
		t.Fatalf("the cast was not held: %v", held.GetCast())
	}
	// What the master reads.
	if got := a.get(t, a.master).GetReactionWait().GetTitlePt(); got != "Esperando a reação de Pensantus" {
		t.Errorf("the master reads %q, want \"Esperando a reação de Pensantus\"", got)
	}
	// What Pensantus's player gets: the prompt without the spell, nor its level.
	w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_COUNTERSPELL)
	if w == nil || !w.GetForYou() || w.GetCounterspell().GetCasterLabel() != "Mago Sombrio" || w.GetTrigger() != nil {
		t.Fatalf("Pensantus's window = %v, want her Counterspell prompt with the caster and nothing of the master's", w)
	}
	if got := len(w.GetCounterspell().GetSlotOptions()); got != 1 || w.GetCounterspell().GetSlotOptions()[0].GetLevel() != 3 {
		t.Errorf("slot options = %v, want the 3rd level alone (a level 5 wizard)", w.GetCounterspell().GetSlotOptions())
	}
	for _, text := range []string{asJSON(t, a.get(t, a.ana)), asJSON(t, a.get(t, a.caio))} {
		if strings.Contains(text, magicMissileSpell) || strings.Contains(text, "Mísseis Mágicos") {
			t.Errorf("a player's combat names the spell before it is countered: %s", text)
		}
	}
	// Toren's player reads only the wait, and no prompt.
	if got := a.get(t, a.caio); len(got.GetReactionWindows()) != 0 || got.GetReactionWait().GetTitlePt() != "Esperando a reação de Pensantus" {
		t.Errorf("Toren's player reads %v / %v", got.GetReactionWindows(), got.GetReactionWait())
	}
	// Toren's player cannot answer for her.
	_, err = a.answerReaction(t, a.caio, e, w.GetId(), passAnswer)
	wantCode(t, "another player answering", err, connect.CodePermissionDenied)

	res := a.mustAnswer(t, a.ana, e, w.GetId(), useAnswer(3))
	c := res.GetResult().GetCounterspell()
	if !c.GetCountered() || c.GetSpellKey() != magicMissileSpell || c.GetSpellLevel() != 1 {
		t.Fatalf("the result = %v, want the spell revealed and countered", res.GetResult())
	}
	if pens := byLabel(t, res.GetEncounter(), "Pensantus"); !pens.GetReactionUsed() {
		t.Error("Pensantus's reaction is not spent")
	}
	if toren := byLabel(t, a.get(t, a.master), "Toren"); toren.GetHitPointsCurrent() != toren.GetHitPointsMax() {
		t.Error("Toren took damage from a countered spell")
	}
	if !byLabel(t, a.get(t, a.master), "Mago Sombrio").GetActionUsed() {
		t.Error("the countered caster's action is not spent")
	}
	// What a player's log says: the reactor, the spell, and nobody else's numbers.
	var line string
	for _, r := range a.log(t, a.caio, e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_REACTION_WINDOW {
				line = en.GetReactionTextPt()
			}
		}
	}
	if !strings.HasPrefix(line, "Pensantus usou Contramágica:") || !strings.Contains(line, "foi anulada") {
		t.Errorf("Toren's player's log line = %q", line)
	}
}

// concentratingHit has the Capitão hit Pensantus, who concentrates on Teia, for the
// damage rolled, lets Escudo go and applies it: the damage that asks the save.
func concentratingHit(t *testing.T, damage int) (*armed, *playv1.Encounter) {
	t.Helper()
	return concentratingHitOn(t, newCasters(t), damage)
}

// concentratingHitOn is concentratingHit on a party the test made.
func concentratingHitOn(t *testing.T, a *armed, damage int) (*armed, *playv1.Encounter) {
	t.Helper()
	e := a.castersFightNPCFirst(t)
	a.concentrate(t, "Pensantus", webSpell)
	hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))
	if _, err := a.declineReaction(t, a.ana, e, hit.GetPendingDamage().GetId()); err != nil {
		t.Fatalf("DeclineReaction() error = %v", err)
	}
	a.h.roller.queue(damage)
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage)
	if _, err := a.settle(t, a.master, e, hit.GetPendingDamage().GetId(), true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	return a, e
}

// TestADamageOnAConcentratingCharacterAsksItsOwnerForTheSave: the damage the master applies
// opens a Constitution saving throw for the owner (DC 10 or half the damage, SRD, Duration),
// the action that dealt it waits, the others read whose save it is, and a failure ends the
// concentration.
func TestADamageOnAConcentratingCharacterAsksItsOwnerForTheSave(t *testing.T) {
	t.Parallel()
	a, e := concentratingHit(t, 6)
	w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE)
	if w == nil {
		t.Fatalf("Pensantus's windows = %v, want a concentration save", a.windows(t, a.ana))
	}
	p := w.GetConcentrationSave()
	// The rule "Os jogadores veem a CD dos testes de resistência" is off: the DC 10 is the master's alone.
	if p.GetSpellKey() != webSpell || p.GetDc() != 0 || p.GetDamageTaken() < 1 || !p.GetBonusKnown() || p.GetSpellNamePt() == "" {
		t.Errorf("the prompt = %v, want Teia, no DC and the damage she took", p)
	}
	if mw := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE); mw.GetConcentrationSave().GetDc() != 10 {
		t.Errorf("the master's prompt = %v, want DC 10", mw.GetConcentrationSave())
	}
	if got := a.get(t, a.caio).GetReactionWait().GetTitlePt(); got != "Esperando o teste de Constituição de Pensantus" {
		t.Errorf("another player reads %q, want the save named", got)
	}
	if len(a.windows(t, a.caio)) != 0 {
		t.Error("another player got the concentration window")
	}
	// The action that dealt the damage waits for it.
	_, err := a.attack(t, a.master, e, "Capitão Goblin", sword, "Toren", d20(9))
	wantBlockedBy(t, "an attack while the save waits", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CONCENTRATION_SAVE_PENDING)
	_, err = a.endTurn(t, a.master, e, false)
	wantBlockedBy(t, "EndTurn while the save waits", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CONCENTRATION_SAVE_PENDING)
	// Not hers to roll for another player.
	_, err = a.resolveConcentration(t, a.caio, e, w.GetId(), concentrationFace(20))
	wantCode(t, "another player's roll", err, connect.CodePermissionDenied)

	res, err := a.resolveConcentration(t, a.ana, e, w.GetId(), concentrationFace(3)) // 3 + 1 = 4 against 10
	if err != nil {
		t.Fatalf("ResolveConcentrationSave() error = %v", err)
	}
	if r := res.GetResult(); r.GetKept() || r.GetDc() != 0 || r.GetSave().GetTotal() != 4 {
		t.Errorf("the result = %v, want a save of 4 that failed, with no DC (the rule is off)", r)
	}
	if got := byLabel(t, res.GetEncounter(), "Pensantus").GetConcentrationSpell(); got != "" {
		t.Errorf("Pensantus still concentrates on %q after a failed save", got)
	}
	var lost bool
	for _, r := range a.log(t, a.caio, e).GetRounds() {
		for _, en := range r.GetEntries() {
			lost = lost || (en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_CONDITIONS_CHANGED && en.GetConcentrationEndedKey() == webSpell)
		}
	}
	if !lost {
		t.Error("the log has no line for the concentration that ended")
	}
	if _, err := a.endTurn(t, a.master, e, false); err != nil {
		t.Errorf("EndTurn() after the save error = %v", err)
	}
}

// TestAConcentrationSaveWithExhaustionTakesTwoPhysicalDice: exhaustion 3 or more is
// disadvantage on saving throws (SRD 5.1), so the prompt says so and a physical roll needs both
// d20; one face is refused and two resolve with the lower.
func TestAConcentrationSaveWithExhaustionTakesTwoPhysicalDice(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFightNPCFirst(t)
	a.concentrate(t, "Pensantus", webSpell)
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE combatants SET exhaustion_level = 3 WHERE id = $1`, a.id(t, "Pensantus")); err != nil {
		t.Fatalf("exhaust Pensantus: %v", err)
	}
	hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", d20(9))
	if _, err := a.declineReaction(t, a.ana, e, hit.GetPendingDamage().GetId()); err != nil {
		t.Fatalf("DeclineReaction() error = %v", err)
	}
	a.h.roller.queue(6)
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage)
	if _, err := a.settle(t, a.master, e, hit.GetPendingDamage().GetId(), true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE)
	if w == nil {
		t.Fatalf("Pensantus's windows = %v, want a concentration save", a.windows(t, a.ana))
	}
	if p := w.GetConcentrationSave(); p.GetMode() != "disadvantage" || len(p.GetModeSourcesPt()) == 0 {
		t.Errorf("the prompt mode = %q %v, want disadvantage with its source", p.GetMode(), p.GetModeSourcesPt())
	}
	_, err := a.resolveConcentration(t, a.ana, e, w.GetId(), concentrationFace(15))
	wantCode(t, "one face with disadvantage", err, connect.CodeInvalidArgument)
	res, err := a.resolveConcentration(t, a.ana, e, w.GetId(), func(r *playv1.ResolveConcentrationSaveRequest) {
		r.Roll = &playv1.ResolveConcentrationSaveRequest_D20Face{D20Face: 15}
		r.SecondD20Face = new(int32(3))
	})
	if err != nil {
		t.Fatalf("ResolveConcentrationSave(15, 3) error = %v", err)
	}
	// The lower die counts: 3 plus the bonus is under the DC 10.
	if r := res.GetResult(); r.GetKept() || r.GetSave().GetTotal() >= 10 {
		t.Errorf("the result = %v, want the lower die (3) to count and the save to fail", r)
	}
}

// TestAConcentrationSaveKeptKeepsTheSpell: a roll that reaches the DC keeps it.
func TestAConcentrationSaveKeptKeepsTheSpell(t *testing.T) {
	t.Parallel()
	a, e := concentratingHit(t, 6)
	w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE)
	res, err := a.resolveConcentration(t, a.ana, e, w.GetId(), concentrationFace(15))
	if err != nil || !res.GetResult().GetKept() {
		t.Fatalf("ResolveConcentrationSave(15) = %v, %v; want the concentration kept", res, err)
	}
	if got := byLabel(t, res.GetEncounter(), "Pensantus").GetConcentrationSpell(); got != webSpell {
		t.Errorf("Pensantus concentrates on %q, want Teia", got)
	}
	// The same answer again is refused: the window is closed.
	_, err = a.resolveConcentration(t, a.ana, e, w.GetId(), concentrationFace(15))
	wantBlockedBy(t, "a second answer", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_REACTION_ALREADY_ANSWERED)
}

// TestAPlayerMayLeaveTheConcentrationSaveToTheMaster: "Deixar o mestre rolar por mim".
func TestAPlayerMayLeaveTheConcentrationSaveToTheMaster(t *testing.T) {
	t.Parallel()
	a, e := concentratingHit(t, 6)
	w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE)
	if _, err := a.resolveConcentration(t, a.ana, e, w.GetId(), func(r *playv1.ResolveConcentrationSaveRequest) {
		r.Roll = &playv1.ResolveConcentrationSaveRequest_HandToMaster{HandToMaster: true}
	}); err != nil {
		t.Fatalf("ResolveConcentrationSave(hand_to_master) error = %v", err)
	}
	if mw := a.windowOf(t, a.master, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE); mw == nil || !mw.GetConcentrationSave().GetHandedToMaster() {
		t.Fatalf("the master's window = %v, want the save left to him", mw)
	}
	_, err := a.resolveConcentration(t, a.ana, e, w.GetId(), concentrationFace(20))
	wantCode(t, "her own roll after leaving it", err, connect.CodePermissionDenied)
	res, err := a.resolveConcentration(t, a.master, e, w.GetId(), func(r *playv1.ResolveConcentrationSaveRequest) {
		r.Roll = &playv1.ResolveConcentrationSaveRequest_RollInApp{RollInApp: true}
	})
	if err != nil {
		t.Fatalf("ResolveConcentrationSave(master) error = %v", err)
	}
	if res.GetResult().GetSave().GetTotal() == 0 {
		t.Errorf("the master's roll = %v", res.GetResult())
	}
}

// TestADamageToZeroEndsTheConcentrationWithoutASave: at 0 hit points the creature is
// incapacitated and loses the concentration (SRD, Duration): no window, no wait.
func TestADamageToZeroEndsTheConcentrationWithoutASave(t *testing.T) {
	t.Parallel()
	a, e := concentratingHit(t, 30)
	if w := a.windowOf(t, a.ana, playv1.ReactionKind_REACTION_KIND_CONCENTRATION_SAVE); w != nil {
		t.Errorf("a save %v waits for a character at 0 hit points", w)
	}
	if got := byLabel(t, a.get(t, a.master), "Pensantus").GetConcentrationSpell(); got != "" {
		t.Errorf("Pensantus concentrates on %q at 0 hit points", got)
	}
	if _, err := a.endTurn(t, a.master, e, false); err != nil {
		t.Errorf("EndTurn() error = %v: nothing waits", err)
	}
}

// markRolling calls MarkReactionRolling as u.
func (a *armed) markRolling(t *testing.T, u *user, e *playv1.Encounter, windowID string) error {
	t.Helper()
	_, err := u.combat.MarkReactionRolling(t.Context(), connect.NewRequest(&playv1.MarkReactionRollingRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), WindowId: windowID}))
	return err
}

// TestMarkingAReactionAsRolledIsTheReactorsAloneAndShowsToTheMaster: the reactor's player marks
// their window (twice is the same as once) and the master reads `rolling`; the master, another
// player and an invented window are refused, the last two with the same answer (RN-10); the mark
// writes no log line and the window is still answered as before.
func TestMarkingAReactionAsRolledIsTheReactorsAloneAndShowsToTheMaster(t *testing.T) {
	t.Parallel()
	r, re := reactorFight(t, rogueFive)
	hit := r.mustAttack(t, r.master, re, "Goblin", sword, "Reator", d20(15))
	r.mustDamage(t, r.master, re, hit.GetPendingDamage().GetId(), typedDamage(6))
	w := r.windowOf(t, r.ana, playv1.ReactionKind_REACTION_KIND_UNCANNY_DODGE)
	if w == nil {
		t.Fatal("the rogue has no Uncanny Dodge window")
	}
	if w.GetRolling() {
		t.Error("the window is marked before anybody rolled")
	}
	fake := "00000000-0000-4000-8000-000000000001"
	wantCode(t, "the master marking", r.markRolling(t, r.master, re, w.GetId()), connect.CodePermissionDenied)
	wantCode(t, "another player marking", r.markRolling(t, r.caio, re, w.GetId()), connect.CodePermissionDenied)
	wantCode(t, "another player marking an invented window", r.markRolling(t, r.caio, re, fake), connect.CodePermissionDenied)
	if got := r.windowOf(t, r.master, playv1.ReactionKind_REACTION_KIND_UNCANNY_DODGE); got.GetRolling() {
		t.Error("a refused mark showed on the master's window")
	}

	logBefore := logLines(r.log(t, r.master, re))
	for range 2 {
		if err := r.markRolling(t, r.ana, re, w.GetId()); err != nil {
			t.Fatalf("the reactor marking the window error = %v", err)
		}
	}
	for who, u := range map[string]*user{"the master": r.master, "the reactor": r.ana} {
		if got := r.windowOf(t, u, playv1.ReactionKind_REACTION_KIND_UNCANNY_DODGE); !got.GetRolling() {
			t.Errorf("%s does not read the window as being rolled", who)
		}
	}
	if n := logLines(r.log(t, r.master, re)); n != logBefore {
		t.Errorf("marking wrote %d log lines", n-logBefore)
	}
	if _, err := r.answerReaction(t, r.ana, re, w.GetId(), useAnswer(0)); err != nil {
		t.Errorf("answering the marked window error = %v", err)
	}
	wantCode(t, "marking an answered window", r.markRolling(t, r.ana, re, w.GetId()), connect.CodeFailedPrecondition)
}

// logLines counts the entries of a combat log.
func logLines(l *playv1.ListCombatLogResponse) int {
	n := 0
	for _, round := range l.GetRounds() {
		n += len(round.GetEntries())
	}
	return n
}
