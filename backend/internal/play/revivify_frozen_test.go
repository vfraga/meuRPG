package play

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps"
)

// Characterization of Revivify inside a combat (revivify.go), written before the function split:
// every refusal with its exact code and message, what each viewer reads from the preview and
// from the list of deaths, and the master's casts. The tests freeze what the code does today;
// a "finding" comment marks a behavior that looks wrong and is kept on purpose.

const (
	revivifyTargetMsg = "that creature cannot be revived now"
	revivifyNoSlotMsg = "slot must be a spell slot of at least the spell's level"
)

// revivifyBase is a valid Revivify cast of Ilaria on label, which edit may break.
func (a *armed) revivifyBase(t *testing.T, target string, edit func(*playv1.CastSpellRequest)) *playv1.CastSpellRequest {
	t.Helper()
	req := &playv1.CastSpellRequest{
		CampaignId: a.campaignID, EncounterId: a.get(t, a.master).GetId(), CasterId: a.id(t, "Ilaria"), SpellKey: revivify, Slot: slotOfLevel(3),
		Targets: a.deadTarget(t, target), MaterialConfirmed: true, IdempotencyKey: newKey(),
	}
	if edit != nil {
		edit(req)
	}
	return req
}

func (a *armed) castRevivifyRaw(t *testing.T, u *user, req *playv1.CastSpellRequest) (*playv1.CastSpellResponse, error) {
	t.Helper()
	res, err := u.combat.CastSpell(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (a *armed) previewRaw(t *testing.T, u *user, req *playv1.PreviewRevivifyRequest) (*playv1.PreviewRevivifyResponse, error) {
	t.Helper()
	res, err := u.revivify.PreviewRevivify(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// revivifyLine is a compact golden of what a preview says: the slot, and each target and each
// reason with its death round and rounds since.
func revivifyLine(pv *playv1.PreviewRevivifyResponse) string {
	var b strings.Builder
	fmt.Fprintf(&b, "slot=%d/%d time=%v targets=[", pv.GetSlot().GetLevel(), pv.GetSlotsFree(), pv.GetCountsTime())
	for i, tg := range pv.GetTargets() {
		if i > 0 {
			b.WriteString(", ")
		}
		since := "nil"
		if tg.RoundsSinceDeath != nil {
			since = fmt.Sprint(tg.GetRoundsSinceDeath())
		}
		round := "nil"
		if tg.DeathRound != nil {
			round = fmt.Sprint(tg.GetDeathRound())
		}
		fmt.Fprintf(&b, "%s r%s s%s", tg.GetName(), round, since)
	}
	b.WriteString("] unavailable=[")
	for i, u := range pv.GetUnavailable() {
		if i > 0 {
			b.WriteString(", ")
		}
		round := "nil"
		if u.DeathRound != nil {
			round = fmt.Sprint(u.GetDeathRound())
		}
		fmt.Fprintf(&b, "%s %s r%s s%d", u.GetName(), strings.TrimPrefix(u.GetReason().String(), "REVIVIFY_UNAVAILABLE_REASON_"), round, u.GetRoundsSinceDeath())
	}
	b.WriteString("]")
	return b.String()
}

func wantRevivifyLine(t *testing.T, who string, pv *playv1.PreviewRevivifyResponse, err error, want string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: PreviewRevivify() error = %v", who, err)
	}
	if got := revivifyLine(pv); got != want {
		t.Errorf("%s reads\n got  %s\n want %s", who, got, want)
	}
}

// The preview's refusals, in the order the function checks them.
func TestRevivifyFrozen_PreviewRefusals(t *testing.T) {
	t.Parallel()
	a, e := newMortuary(t)
	ilaria := a.id(t, "Ilaria")
	call := func(u *user, enc, caster string) error {
		t.Helper()
		_, err := a.previewRaw(t, u, &playv1.PreviewRevivifyRequest{CampaignId: a.campaignID, EncounterId: enc, CasterId: caster})
		return err
	}
	wantRefusal(t, "a bad encounter id", call(a.ana, "nope", ilaria), connect.CodeNotFound, "encounter not found")
	wantRefusal(t, "an encounter that is not there", call(a.ana, newKey(), ilaria), connect.CodeNotFound, "encounter not found")
	wantRefusal(t, "a bad caster id", call(a.ana, e.GetId(), "nope"), connect.CodeNotFound, "combatant not found")
	wantRefusal(t, "a caster that is not there", call(a.ana, e.GetId(), newKey()), connect.CodeNotFound, "combatant not found")
	wantRefusal(t, "another player's caster", call(a.caio, e.GetId(), ilaria), connect.CodePermissionDenied, "only the combatant's player or the master may do this")
	wantRefusal(t, "a caster with no Revivify", call(a.bia, e.GetId(), a.id(t, "Brisa")), connect.CodeInvalidArgument, "spell_key is not one of the caster's spells")
	wantRefusal(t, "the master asking for a caster with no Revivify", call(a.master, e.GetId(), a.id(t, "Brisa")), connect.CodeInvalidArgument, "spell_key is not one of the caster's spells")

	// A combat that ended answers the same to everybody who asks.
	if _, err := a.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	for who, u := range map[string]*user{"the player": a.ana, "the master": a.master} {
		err := call(u, e.GetId(), ilaria)
		wantRefusal(t, who+" asking after the end", err, connect.CodeFailedPrecondition, "the combat has ended")
		wantBlockedBy(t, who+" asking after the end", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ENCOUNTER_ENDED)
	}
}

// What each viewer reads from the preview, with two dead creatures, then with no slot left, then
// with the death out of its window.
func TestRevivifyFrozen_PreviewIsWhatEachViewerMayRead(t *testing.T) {
	t.Parallel()
	a, e := newMortuary(t)
	a.dies(t, e, "Toren", a.toren)
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(99)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	both := "slot=3/2 time=true targets=[Toren r1 s0, Goblin r1 s0] unavailable=[]"
	pv, err := a.preview(t, a.ana, e, "Ilaria")
	wantRevivifyLine(t, "the caster's player", pv, err, both)
	pv, err = a.preview(t, a.master, e, "Ilaria")
	wantRevivifyLine(t, "the master", pv, err, both)

	// The reasons are the master's: the goblin is hidden, Toren marked, and the player reads
	// neither of them.
	if _, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Goblin"), IdempotencyKey: newKey(), Hidden: true,
	})); err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}
	if _, err := a.master.revivify.SetRevivifyBlocked(t.Context(), connect.NewRequest(&playv1.SetRevivifyBlockedRequest{
		CampaignId: a.campaignID, Target: &playv1.SetRevivifyBlockedRequest_CombatantId{CombatantId: a.id(t, "Toren")}, Blocked: true,
	})); err != nil {
		t.Fatalf("SetRevivifyBlocked() error = %v", err)
	}
	pv, err = a.preview(t, a.ana, e, "Ilaria")
	wantRevivifyLine(t, "the caster's player, nothing to cast on", pv, err, "slot=3/2 time=true targets=[] unavailable=[]")
	pv, err = a.preview(t, a.master, e, "Ilaria")
	wantRevivifyLine(t, "the master, with the reasons", pv, err, "slot=3/2 time=true targets=[] unavailable=[Toren MASTER_BLOCKED r1 s0, Goblin HIDDEN r1 s0]")

	// No free 3rd level slot: the lists stay, the slot is gone.
	a.correct(t, a.pens, func(r *playv1.AdjustCharacterVitalsRequest) {
		r.SpellSlotsUsed = []*playv1.SpellSlotsUsed{{Level: 3, Used: 2}}
	})
	pv, err = a.preview(t, a.master, e, "Ilaria")
	wantRevivifyLine(t, "the master with every slot spent", pv, err, "slot=0/0 time=true targets=[] unavailable=[Toren MASTER_BLOCKED r1 s0, Goblin HIDDEN r1 s0]")
	if pv.GetSlot() != nil {
		t.Errorf("the slot with none free = %v, want nil", pv.GetSlot())
	}
}

// A death with no round is always in time (it is from before the round was kept); a death in the
// last round counts up to its place in the order, and when nobody is on turn or the place was not
// kept it counts as in time.
func TestRevivifyFrozen_TheWindowAtItsEdges(t *testing.T) {
	t.Parallel()
	a, e := newMortuary(t)
	a.dies(t, e, "Toren", a.toren)
	toren := a.id(t, "Toren")
	line := func(u *user) string {
		t.Helper()
		pv, err := a.preview(t, u, e, "Ilaria")
		if err != nil {
			t.Fatalf("PreviewRevivify() error = %v", err)
		}
		return revivifyLine(pv)
	}
	a.execSQL(t, `UPDATE encounters SET round = 40 WHERE id = $1`, e.GetId())
	if got, want := line(a.master), "slot=3/2 time=true targets=[] unavailable=[Toren TOO_LONG_AGO r1 s39]"; got != want {
		t.Errorf("39 rounds after the death: %s, want %s", got, want)
	}
	a.execSQL(t, `UPDATE combatants SET death_round = NULL WHERE id = $1`, toren)
	// Finding: a death with no round reads rounds_since_death 0 (revivify.go, windowOf and
	// PreviewRevivify), as if it were this round, where the round itself is absent.
	if got, want := line(a.ana), "slot=3/2 time=true targets=[Toren rnil s0] unavailable=[]"; got != want {
		t.Errorf("a death with no round: %s, want %s", got, want)
	}
	// The list of deaths leaves out a death that has no round.
	if got := deathEntry(a.get(t, a.master), "Toren", toren); got != nil {
		t.Errorf("the master's deaths hold %v for a death with no round, want none", got)
	}

	a.execSQL(t, `UPDATE combatants SET death_round = 1, death_order_index = NULL WHERE id = $1`, toren)
	a.execSQL(t, `UPDATE encounters SET round = 11 WHERE id = $1`, e.GetId())
	if got, want := line(a.ana), "slot=3/2 time=true targets=[Toren r1 s10] unavailable=[]"; got != want {
		t.Errorf("10 rounds after, with no place in the order: %s, want %s", got, want)
	}
	a.execSQL(t, `UPDATE combatants SET death_order_index = 0 WHERE id = $1`, toren)
	if got, want := line(a.ana), "slot=3/2 time=true targets=[] unavailable=[]"; got != want {
		t.Errorf("10 rounds after, past the place in the order: %s, want %s", got, want)
	}
	a.execSQL(t, `UPDATE encounters SET current_combatant_id = NULL WHERE id = $1`, e.GetId())
	if got, want := line(a.ana), "slot=3/2 time=true targets=[Toren r1 s10] unavailable=[]"; got != want {
		t.Errorf("10 rounds after, with nobody on turn: %s, want %s", got, want)
	}
}

// The list of deaths: the master reads every death in the order they happened, with the window
// and his switch; a player reads their own character and the round, and nothing else.
func TestRevivifyFrozen_DeathsOrderAndWhoReadsThem(t *testing.T) {
	t.Parallel()
	a, e := newMortuary(t)
	a.dies(t, e, "Toren", a.toren)
	a.dies(t, e, "Brisa", a.bri)
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(99)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	toren, brisa, goblin := a.id(t, "Toren"), a.id(t, "Brisa"), a.id(t, "Goblin")
	// Same round for all three: the place in the order breaks the tie; the goblin died later.
	a.execSQL(t, `UPDATE combatants SET death_round = 1, death_order_index = 5 WHERE id = $1`, brisa)
	a.execSQL(t, `UPDATE combatants SET death_round = 1, death_order_index = 2 WHERE id = $1`, toren)
	a.execSQL(t, `UPDATE combatants SET death_round = 2, death_order_index = 0 WHERE id = $1`, goblin)
	a.execSQL(t, `UPDATE encounters SET round = 12 WHERE id = $1`, e.GetId())

	read := func(u *user) string {
		t.Helper()
		var parts []string
		for _, d := range a.get(t, u).GetDeaths() {
			fits, blocked := "nil", "nil"
			if d.FitsRevivify != nil {
				fits = fmt.Sprint(d.GetFitsRevivify())
			}
			if d.RevivifyBlocked != nil {
				blocked = fmt.Sprint(d.GetRevivifyBlocked())
			}
			parts = append(parts, fmt.Sprintf("%s r%d pc=%v char=%v fits=%s blocked=%s", d.GetName(), d.GetDeathRound(), d.GetIsPlayerCharacter(), d.GetCharacterId() != "", fits, blocked))
		}
		return strings.Join(parts, "; ")
	}
	wantMaster := "Toren r1 pc=true char=true fits=false blocked=false; Brisa r1 pc=true char=true fits=false blocked=false; Goblin r2 pc=false char=false fits=false blocked=false"
	if got := read(a.master); got != wantMaster {
		t.Errorf("the master's deaths\n got  %s\n want %s", got, wantMaster)
	}
	if got, want := read(a.caio), "Toren r1 pc=true char=true fits=nil blocked=nil"; got != want {
		t.Errorf("Toren's player reads %s, want %s", got, want)
	}
	if got, want := read(a.bia), "Brisa r1 pc=true char=true fits=nil blocked=nil"; got != want {
		t.Errorf("Brisa's player reads %s, want %s", got, want)
	}
	if got := read(a.ana); got != "" {
		t.Errorf("Ilaria's player reads the deaths %s, want none", got)
	}
}

// The cast's shape and the refusals before anything is written. The same request goes through a
// player and the master where it matters.
func TestRevivifyFrozen_CastRefusals(t *testing.T) {
	t.Parallel()
	a, e := newMortuary(t)
	a.dies(t, e, "Toren", a.toren)
	e = a.passTo(t, e, "Ilaria")

	shape := func(call string, edit func(*playv1.CastSpellRequest), code connect.Code, msg string) {
		t.Helper()
		_, err := a.castRevivifyRaw(t, a.ana, a.revivifyBase(t, "Toren", edit))
		wantRefusal(t, call, err, code, msg)
	}
	const one = "Revivify takes one target: the dead creature, as dead_target_id"
	shape("a bad key", func(r *playv1.CastSpellRequest) { r.IdempotencyKey = "nope" }, connect.CodeInvalidArgument, "idempotency_key must be a UUID")
	shape("a bad encounter", func(r *playv1.CastSpellRequest) { r.EncounterId = "nope" }, connect.CodeNotFound, "encounter not found")
	shape("a bad caster", func(r *playv1.CastSpellRequest) { r.CasterId = "nope" }, connect.CodeNotFound, "combatant not found")
	shape("no target", func(r *playv1.CastSpellRequest) { r.Targets = nil }, connect.CodeInvalidArgument, one)
	shape("two targets", func(r *playv1.CastSpellRequest) {
		r.Targets = append(r.Targets, &playv1.SpellTarget{DeadTargetId: a.id(t, "Brisa")})
	}, connect.CodeInvalidArgument, one)
	shape("an empty dead_target_id", func(r *playv1.CastSpellRequest) { r.Targets = []*playv1.SpellTarget{{}} }, connect.CodeInvalidArgument, one)
	shape("a combatant_id besides", func(r *playv1.CastSpellRequest) { r.Targets[0].CombatantId = a.id(t, "Toren") }, connect.CodeInvalidArgument, one)
	shape("darts", func(r *playv1.CastSpellRequest) { r.Targets[0].Darts = 1 }, connect.CodeInvalidArgument, one)
	shape("a dead_target_id that is not an id", func(r *playv1.CastSpellRequest) { r.Targets[0].DeadTargetId = "toren" }, connect.CodeFailedPrecondition, revivifyTargetMsg)
	shape("no diamonds", func(r *playv1.CastSpellRequest) { r.MaterialConfirmed = false }, connect.CodeInvalidArgument,
		"material_confirmed is required: the spell consumes diamonds worth 300 gp")
	const rolls = "Revivify rolls nothing and takes no summon or damage type"
	shape("a roll", func(r *playv1.CastSpellRequest) { r.Roll = &playv1.CastSpellRequest_RollInApp{RollInApp: true} }, connect.CodeInvalidArgument, rolls)
	shape("a summon", func(r *playv1.CastSpellRequest) { r.Summon = &playv1.SummonChoice{} }, connect.CodeInvalidArgument, rolls)
	shape("a damage type", func(r *playv1.CastSpellRequest) { r.DamageTypeKey = "damage:fire" }, connect.CodeInvalidArgument, rolls)

	// Who may cast, and the options the caster has.
	_, err := a.castRevivifyRaw(t, a.caio, a.revivifyBase(t, "Toren", nil))
	wantRefusal(t, "another player's caster", err, connect.CodePermissionDenied, "only the combatant's player or the master may do this")
	_, err = a.castRevivifyRaw(t, a.ana, a.revivifyBase(t, "Toren", func(r *playv1.CastSpellRequest) { r.CasterId = newKey() }))
	wantRefusal(t, "a caster that is not there", err, connect.CodeNotFound, "combatant not found")
	slot := func(call string, s *playv1.SpellSlot, msg string) {
		t.Helper()
		_, err := a.castRevivifyRaw(t, a.ana, a.revivifyBase(t, "Toren", func(r *playv1.CastSpellRequest) { r.Slot = s }))
		wantRefusal(t, call, err, connect.CodeInvalidArgument, msg)
	}
	slot("no slot", nil, revivifyNoSlotMsg)
	slot("a 2nd level slot", slotOfLevel(2), revivifyNoSlotMsg)
	slot("a slot of a level the spell does not have", slotOfLevel(10), revivifyNoSlotMsg)
	slot("a pact slot the caster does not have", &playv1.SpellSlot{Level: 3, Pact: true}, "slot is not one the caster has free")
	slot("a 4th level slot the caster does not have", slotOfLevel(4), "slot is not one the caster has free")

	// The action is used: a player is refused, the master is not stopped by it.
	a.execSQL(t, `UPDATE combatants SET action_used = true WHERE id = $1`, a.id(t, "Ilaria"))
	_, err = a.castRevivifyRaw(t, a.ana, a.revivifyBase(t, "Toren", nil))
	wantRefusal(t, "the action used", err, connect.CodeFailedPrecondition, "the action of this turn is used")
	wantBlockedBy(t, "the action used", err, blockedActionUsed)
	a.execSQL(t, `UPDATE combatants SET action_used = false WHERE id = $1`, a.id(t, "Ilaria"))

	// No free slot of the 3rd level.
	a.correct(t, a.pens, func(r *playv1.AdjustCharacterVitalsRequest) {
		r.SpellSlotsUsed = []*playv1.SpellSlotsUsed{{Level: 3, Used: 2}}
	})
	_, err = a.castRevivifyRaw(t, a.ana, a.revivifyBase(t, "Toren", nil))
	wantRefusal(t, "no slot left", err, connect.CodeFailedPrecondition, "there is no free spell slot for this spell")
	if b := wantBlockedBy(t, "no slot left", err, playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_SLOT); b.GetMinLevel() != 3 {
		t.Errorf("NO_SLOT min_level = %d, want 3", b.GetMinLevel())
	}
	a.correct(t, a.pens, func(r *playv1.AdjustCharacterVitalsRequest) {
		r.SpellSlotsUsed = []*playv1.SpellSlotsUsed{{Level: 3, Used: 0}}
	})

	// Not Ilaria's turn, and a caster that has no Revivify on its own turn.
	e = a.passTo(t, e, "Brisa")
	_, err = a.castRevivifyRaw(t, a.ana, a.revivifyBase(t, "Toren", nil))
	wantRefusal(t, "out of turn", err, connect.CodeFailedPrecondition, "it is not this combatant's turn")
	wantBlockedBy(t, "out of turn", err, reasonNotTurn)
	_, err = a.castRevivifyRaw(t, a.bia, a.revivifyBase(t, "Toren", func(r *playv1.CastSpellRequest) { r.CasterId = a.id(t, "Brisa") }))
	wantRefusal(t, "a caster with no Revivify", err, connect.CodeInvalidArgument, "spell_key is not one of the caster's spells")
	if v := a.vitals(t, a.pens); usedSlots(v, 3) != 0 || byLabel(t, a.get(t, a.master), "Ilaria").GetActionUsed() {
		t.Errorf("a refused cast spent something: slots %d", usedSlots(v, 3))
	}

	// A combat that ended.
	if _, err := a.master.combat.EndEncounter(t.Context(), connect.NewRequest(&playv1.EndEncounterRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("EndEncounter() error = %v", err)
	}
	_, err = a.castRevivifyRaw(t, a.ana, a.revivifyBase(t, "Toren", nil))
	wantRefusal(t, "a cast after the end", err, connect.CodeFailedPrecondition, "the combat has ended")
}

// The master casts for a player's character: he is held to the window and to his switch, and to
// nothing else (not the range, not what is hidden, not the action already used).
func TestRevivifyFrozen_TheMastersCastIsHeldToTheWindowAndHisSwitchOnly(t *testing.T) {
	t.Parallel()
	a, e := newMortuary(t)
	a.dies(t, e, "Toren", a.toren)
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(99)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	if _, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Goblin"), IdempotencyKey: newKey(), Hidden: true,
	})); err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}
	if _, err := a.master.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Ilaria"), IdempotencyKey: newKey(), Col: 9, Row: 9,
	})); err != nil {
		t.Fatalf("MoveCombatant() error = %v", err)
	}
	e = a.passTo(t, e, "Ilaria")
	a.execSQL(t, `UPDATE combatants SET action_used = true WHERE id = $1`, a.id(t, "Ilaria"))

	// The player cannot touch either: far, and one is hidden.
	for _, who := range []string{"Toren", "Goblin"} {
		_, err := a.castRevivifyRaw(t, a.ana, a.revivifyBase(t, who, nil))
		wantRefusal(t, "the player's cast on "+who, err, connect.CodeFailedPrecondition, "the action of this turn is used")
	}
	a.execSQL(t, `UPDATE combatants SET action_used = false WHERE id = $1`, a.id(t, "Ilaria"))
	for _, who := range []string{"Toren", "Goblin"} {
		_, err := a.castRevivifyRaw(t, a.ana, a.revivifyBase(t, who, nil))
		wantRefusal(t, "the player's cast on "+who, err, connect.CodeFailedPrecondition, revivifyTargetMsg)
	}
	a.execSQL(t, `UPDATE combatants SET action_used = true WHERE id = $1`, a.id(t, "Ilaria"))

	// The master revives the hidden goblin from afar with the action already used.
	if _, err := a.castRevivifyRaw(t, a.master, a.revivifyBase(t, "Goblin", nil)); err != nil {
		t.Fatalf("the master's Revivify on the hidden goblin from afar error = %v", err)
	}
	if cur, _, defeated := a.hp(t, "Goblin"); cur != 1 || defeated {
		t.Errorf("the goblin = %d hit points, defeated %v; want 1 and alive", cur, defeated)
	}
	// A cast with a hidden target is a secret line: the players do not read it.
	for who, u := range map[string]*user{"Ilaria's player": a.ana, "Brisa's player": a.bia} {
		for _, r := range a.log(t, u, a.get(t, u)).GetRounds() {
			for _, en := range r.GetEntries() {
				if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST && en.GetKey() == revivify {
					t.Errorf("%s reads the secret Revivify line %v", who, en)
				}
			}
		}
	}

	// The master's own switch and the window stop him; the second cast is free to try again.
	if _, err := a.master.revivify.SetRevivifyBlocked(t.Context(), connect.NewRequest(&playv1.SetRevivifyBlockedRequest{
		CampaignId: a.campaignID, Target: &playv1.SetRevivifyBlockedRequest_CombatantId{CombatantId: a.id(t, "Toren")}, Blocked: true,
	})); err != nil {
		t.Fatalf("SetRevivifyBlocked() error = %v", err)
	}
	_, err := a.castRevivifyRaw(t, a.master, a.revivifyBase(t, "Toren", nil))
	wantRefusal(t, "the master's cast on a marked death", err, connect.CodeFailedPrecondition, revivifyTargetMsg)
	if _, err := a.master.revivify.SetRevivifyBlocked(t.Context(), connect.NewRequest(&playv1.SetRevivifyBlockedRequest{
		CampaignId: a.campaignID, Target: &playv1.SetRevivifyBlockedRequest_CombatantId{CombatantId: a.id(t, "Toren")}, Blocked: false,
	})); err != nil {
		t.Fatalf("SetRevivifyBlocked(false) error = %v", err)
	}
	a.execSQL(t, `UPDATE encounters SET round = 30 WHERE id = $1`, e.GetId())
	_, err = a.castRevivifyRaw(t, a.master, a.revivifyBase(t, "Toren", nil))
	wantRefusal(t, "the master's cast past the window", err, connect.CodeFailedPrecondition, revivifyTargetMsg)
	a.execSQL(t, `UPDATE encounters SET round = 3 WHERE id = $1`, e.GetId())
	if _, err := a.castRevivifyRaw(t, a.master, a.revivifyBase(t, "Toren", nil)); err != nil {
		t.Fatalf("the master's Revivify on Toren from afar error = %v", err)
	}
	if v := a.vitals(t, a.toren); v.GetHitPointsCurrent() != 1 {
		t.Errorf("Toren's hit points = %d, want 1", v.GetHitPointsCurrent())
	}
	if v := a.vitals(t, a.pens); usedSlots(v, 3) != 2 {
		t.Errorf("3rd level slots used by two master casts = %d, want 2", usedSlots(v, 3))
	}
}

// In a combat without a map the master judges the range: a creature stands nowhere and is still
// listed and reached.
func TestRevivifyFrozen_TheatreHasNoRange(t *testing.T) {
	t.Parallel()
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 4,
			&rulesv1.AbilityScores{Strength: 16, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{battleaxe}, nil)
		a.pens = a.ana.ilaria(t, a.campaignID)
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2,
			&rulesv1.AbilityScores{Strength: 10, Dexterity: 16, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8}, []string{rapier}, nil)
	})
	e := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{3},
		players: map[string]int32{"Toren": 18, "Ilaria": 12, "Brisa": 1}, reveal: []string{"Goblin"}, theatre: true,
	})
	a.dies(t, e, "Toren", a.toren)
	pv, err := a.preview(t, a.ana, e, "Ilaria")
	wantRevivifyLine(t, "the caster's player in theatre", pv, err, "slot=3/2 time=true targets=[Toren r1 s0] unavailable=[]")
	e = a.passTo(t, e, "Ilaria")
	if _, err := a.castRevivify(t, a.ana, e, "Ilaria", "Toren"); err != nil {
		t.Fatalf("Revivify in theatre error = %v", err)
	}
	if v := a.vitals(t, a.toren); v.GetHitPointsCurrent() != 1 {
		t.Errorf("Toren's hit points = %d, want 1", v.GetHitPointsCurrent())
	}
}

// With the fog on, a dead NPC in the dark is not seen by the caster's player: the player's list
// leaves it out and a cast on it gets the one answer; the master reads NOT_SEEN as the reason
// (for the player the caster belongs to) and is not held to what is seen.
func TestRevivifyFrozen_FogHidesAnNPCInTheDarkFromThePlayer(t *testing.T) {
	t.Parallel()
	a, e := newMortuary(t)
	a.dies(t, e, "Toren", a.toren)
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(99)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	content, err := testRules()
	if err != nil {
		t.Fatalf("rules.LoadSRD() error = %v", err)
	}
	msvc, err := maps.New(maps.Config{Pool: a.h.pool, Characters: a.h.chars, Live: a.h.svc, Rules: content, Combats: a.h.svc, Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("maps.New() error = %v", err)
	}
	msvc.SetSessionHost(a.h.svc)
	a.h.svc.SetTerrain(msvc)
	a.h.svc.SetFog(msvc)
	a.execSQL(t, `UPDATE maps SET fog_enabled = true WHERE id = $1`, a.mapID) // the base light is dark

	pv, err := a.preview(t, a.ana, e, "Ilaria")
	wantRevivifyLine(t, "the caster's player in the dark", pv, err, "slot=3/2 time=true targets=[Toren r1 s0] unavailable=[]")
	pv, err = a.preview(t, a.master, e, "Ilaria")
	wantRevivifyLine(t, "the master", pv, err, "slot=3/2 time=true targets=[Toren r1 s0] unavailable=[Goblin NOT_SEEN r1 s0]")

	a.passTo(t, e, "Ilaria")
	_, err = a.castRevivifyRaw(t, a.ana, a.revivifyBase(t, "Goblin", nil))
	wantRefusal(t, "the player's cast on a creature in the dark", err, connect.CodeFailedPrecondition, revivifyTargetMsg)
	if _, err := a.castRevivifyRaw(t, a.master, a.revivifyBase(t, "Goblin", nil)); err != nil {
		t.Fatalf("the master's Revivify on the goblin in the dark error = %v", err)
	}
	if cur, _, defeated := a.hp(t, "Goblin"); cur != 1 || defeated {
		t.Errorf("the goblin = %d hit points, defeated %v; want 1 and alive", cur, defeated)
	}
}
