package play

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// The actions of a turn (MR-012, MR-014, Etapa 6, slice 6.4a): the options of a
// combatant, the attack in two steps, the damage, the standard actions, the
// NPCs' hit points, the undo and the combat log. These tests need the
// database (MEURPG_TEST_DATABASE_URL). The fixture is the party of the
// canonical fight (scratch timeline): Toren, Pensantus and Brisa against the
// Capitão Goblin and the goblins, with the sheets' numbers: Toren +5 to hit
// and 1d8+3, Pensantus's Raio de Fogo +6 and 1d10, Brisa's Rapieira +5 and
// 1d8+3, the NPCs +4 and 1d6+2.

// actionRPCs are the CombatService methods of this slice; the authorization
// matrix of the encounter (combat_test.go) leaves them to this file's.
var actionRPCs = []string{
	"GetTurnOptions", "RollAttack", "RollDamage", "ApplyPendingDamage", "DiscardPendingDamage",
	"TakeAction", "AdjustCombatantHitPoints", "UndoLastAction", "ListCombatLog",
}

// armed is a campaign ready to fight, with weapons: a master and three players
// (Toren, Pensantus and Brisa), the Capitão Goblin (CA 18, 27 PV) and a goblin
// (CA 12, 7 PV), and a map with a grid. The session is open.
type armed struct {
	h                *harness
	campaignID       string
	master           *user
	caio, ana, bia   *user // Toren's, Pensantus's and Brisa's players
	toren, pens, bri *charactersv1.Character
	capitao, goblin  *charactersv1.Character
	mapID            string
}

// hero creates a player's character with a full sheet.
func (u *user) hero(t *testing.T, campaignID, name, class, race string, level int32, scores *rulesv1.AbilityScores, weapons, cantrips []string) *charactersv1.Character {
	t.Helper()
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: scores, RaceKey: race, Classes: []*charactersv1.ClassLevel{{ClassKey: class, Level: level}},
		WeaponKeys: weapons, CantripKeys: cantrips,
	}}}
	res, err := u.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: name, Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(%s) error = %v", name, err)
	}
	return res.Msg.GetCharacter()
}

// npc creates a basic NPC with a sword at +4 (1d6+2, "basic:0") and a short
// bow at 80 ft (1d6+2, "basic:1").
func (u *user) npc(t *testing.T, campaignID, name string, hp, ac int32) *charactersv1.Character {
	t.Helper()
	attack := func(name string, rangeFt int32, t charactersv1.DamageType) *charactersv1.BasicAttack {
		return &charactersv1.BasicAttack{Name: name, AttackBonus: 4, DamageDiceCount: 1, DamageDiceSides: 6, DamageBonus: 2, DamageType: t, RangeFt: rangeFt}
	}
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Basic{Basic: &charactersv1.BasicSheet{
		HitPointsMax: hp, ArmorClass: ac, SpeedFt: 30,
		Attacks: []*charactersv1.BasicAttack{
			attack("Cimitarra", 0, charactersv1.DamageType_DAMAGE_TYPE_SLASHING),
			attack("Arco curto", 80, charactersv1.DamageType_DAMAGE_TYPE_PIERCING),
		},
	}}}
	res, err := u.characters.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaignID, Kind: charactersv1.CharacterKind_CHARACTER_KIND_MINION, Name: name, Sheet: sheet,
	}))
	if err != nil {
		t.Fatalf("CreateCharacter(%s) error = %v", name, err)
	}
	return res.Msg.GetCharacter()
}

const (
	battleaxe = "equipment:battleaxe"
	rapier    = "equipment:rapier"
	fireBolt  = "spell:fire-bolt"
	sword     = "basic:0"
	shortBow  = "basic:1"
)

func newArmed(t *testing.T) *armed {
	t.Helper()
	return newArmedWith(t, func(a *armed) {
		scores := func(str, dex, con, intl int32) *rulesv1.AbilityScores {
			return &rulesv1.AbilityScores{Strength: str, Dexterity: dex, Constitution: con, Intelligence: intl, Wisdom: 10, Charisma: 8}
		}
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2, scores(16, 13, 14, 10), []string{battleaxe}, nil)
		a.pens = a.ana.hero(t, a.campaignID, "Pensantus", "class:wizard", "race:gnome", 1, scores(10, 14, 12, 16), nil, []string{fireBolt})
		a.bri = a.bia.hero(t, a.campaignID, "Brisa", "class:fighter", "race:human", 2, scores(10, 16, 14, 10), []string{rapier}, nil)
	})
}

// newArmedWith is newArmed with the three heroes made by heroes: the NPCs, the
// map and the open session are the same.
func newArmedWith(t *testing.T, heroes func(a *armed)) *armed {
	t.Helper()
	h := newHarness(t)
	a := &armed{h: h, master: h.newUser("Samuel"), caio: h.newUser("Caio"), ana: h.newUser("Ana"), bia: h.newUser("Bia")}
	a.campaignID = h.newCampaign(a.master, "Mirathel", a.caio, a.ana, a.bia)
	heroes(a)
	a.capitao = a.master.npc(t, a.campaignID, "Capitão Goblin", 27, 18)
	a.goblin = a.master.npc(t, a.campaignID, "Goblin", 7, 12)
	a.master.start(t, a.campaignID)
	a.mapID = h.newMap(a.campaignID, gridColumns)
	if _, err := a.master.play.SetCurrentMap(t.Context(), connect.NewRequest(&playv1.SetCurrentMapRequest{CampaignId: a.campaignID, MapId: a.mapID})); err != nil {
		t.Fatalf("SetCurrentMap() error = %v", err)
	}
	return a
}

// plan is how a combat starts: who comes besides the party, what the NPCs roll
// (in the order they are created), the players' d20 (typed by the master), who
// is revealed, and where each one stands.
type plan struct {
	npcs     []*playv1.Participant
	npcRolls []int
	players  map[string]int32 // label -> the d20 face
	reveal   []string
	at       map[string][2]int32
	// setup leaves the combat in SETUP, for the tests that look at it before.
	setup bool
	// theatre starts a combat without a map (RN-25): nobody has a square, so at
	// must be empty.
	theatre bool
}

// setPhysical makes the player roll real dice (RN-18).
func (a *armed) setPhysical(t *testing.T, u *user) {
	t.Helper()
	if _, err := u.campaigns.SetMyDicePreference(t.Context(), connect.NewRequest(&campaignsv1.SetMyDicePreferenceRequest{
		CampaignId: a.campaignID, Preference: campaignsv1.DicePreference_DICE_PREFERENCE_PHYSICAL,
	})); err != nil {
		t.Fatalf("SetMyDicePreference() error = %v", err)
	}
}

// forceDice sets the campaign's dice mode as the master (RN-18).
func (a *armed) forceDice(t *testing.T, mode campaignsv1.DiceMode) {
	t.Helper()
	if _, err := a.master.campaigns.SetCampaignDiceMode(t.Context(), connect.NewRequest(&campaignsv1.SetCampaignDiceModeRequest{CampaignId: a.campaignID, Mode: mode})); err != nil {
		t.Fatalf("SetCampaignDiceMode() error = %v", err)
	}
}

// start runs the whole setup of a combat and begins it, and returns the master's
// copy.
func (a *armed) start(t *testing.T, p plan) *playv1.Encounter {
	t.Helper()
	ctx := t.Context()
	a.h.roller.queue(p.npcRolls...)
	mode := playv1.EncounterMode_ENCOUNTER_MODE_UNSPECIFIED
	if p.theatre {
		mode = playv1.EncounterMode_ENCOUNTER_MODE_THEATRE
	}
	res, err := a.master.combat.StartEncounter(ctx, connect.NewRequest(&playv1.StartEncounterRequest{
		CampaignId: a.campaignID, IdempotencyKey: newKey(), Name: "Emboscada na estrada", Participants: p.npcs, Mode: mode,
	}))
	if err != nil {
		t.Fatalf("StartEncounter() error = %v", err)
	}
	e := res.Msg.GetEncounter()
	for label, face := range p.players {
		if _, err := a.master.combat.SubmitInitiative(ctx, connect.NewRequest(&playv1.SubmitInitiativeRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: byLabel(t, e, label).GetId(), IdempotencyKey: newKey(),
			Roll: &playv1.SubmitInitiativeRequest_D20Face{D20Face: face},
		})); err != nil {
			t.Fatalf("SubmitInitiative(%s) error = %v", label, err)
		}
	}
	for _, label := range p.reveal {
		if _, err := a.master.combat.SetCombatantHidden(ctx, connect.NewRequest(&playv1.SetCombatantHiddenRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: byLabel(t, e, label).GetId(), IdempotencyKey: newKey(), Hidden: false,
		})); err != nil {
			t.Fatalf("SetCombatantHidden(%s) error = %v", label, err)
		}
	}
	for label, sq := range p.at {
		if _, err := a.master.combat.MoveCombatant(ctx, connect.NewRequest(&playv1.MoveCombatantRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: byLabel(t, e, label).GetId(), IdempotencyKey: newKey(), Col: sq[0], Row: sq[1],
		})); err != nil {
			t.Fatalf("MoveCombatant(%s) error = %v", label, err)
		}
	}
	if p.setup {
		return a.get(t, a.master)
	}
	begun, err := a.master.combat.BeginCombat(ctx, connect.NewRequest(&playv1.BeginCombatRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey()}))
	if err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	return begun.Msg.GetEncounter()
}

// threeAndAGoblin is the usual start: Toren first, then Pensantus, then the
// goblin (revealed) and Brisa, with Toren next to the goblin.
func (a *armed) threeAndAGoblin(t *testing.T) *playv1.Encounter {
	t.Helper()
	return a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
}

func (a *armed) get(t *testing.T, u *user) *playv1.Encounter {
	t.Helper()
	res, err := u.combat.GetEncounter(t.Context(), connect.NewRequest(&playv1.GetEncounterRequest{CampaignId: a.campaignID}))
	if err != nil {
		t.Fatalf("GetEncounter() error = %v", err)
	}
	return res.Msg.GetEncounter()
}

// id is a combatant's ID by its label, as the master sees the combat.
func (a *armed) id(t *testing.T, label string) string {
	t.Helper()
	return byLabel(t, a.get(t, a.master), label).GetId()
}

func inAppRoll(r *playv1.RollAttackRequest) {
	r.Roll = &playv1.RollAttackRequest_RollInApp{RollInApp: true}
}

func d20(face int32) func(*playv1.RollAttackRequest) {
	return func(r *playv1.RollAttackRequest) { r.Roll = &playv1.RollAttackRequest_D20Face{D20Face: face} }
}

// attack calls RollAttack as u, by labels.
func (a *armed) attack(t *testing.T, u *user, e *playv1.Encounter, attacker, key, target string, roll func(*playv1.RollAttackRequest)) (*playv1.RollAttackResponse, error) {
	t.Helper()
	req := &playv1.RollAttackRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), AttackerId: a.id(t, attacker), AttackKey: key, TargetId: a.id(t, target), IdempotencyKey: newKey(),
	}
	roll(req)
	res, err := u.combat.RollAttack(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (a *armed) mustAttack(t *testing.T, u *user, e *playv1.Encounter, attacker, key, target string, roll func(*playv1.RollAttackRequest)) *playv1.RollAttackResponse {
	t.Helper()
	res, err := a.attack(t, u, e, attacker, key, target, roll)
	if err != nil {
		t.Fatalf("RollAttack(%s -> %s) error = %v", attacker, target, err)
	}
	return res
}

func inAppDamage(r *playv1.RollDamageRequest) {
	r.Roll = &playv1.RollDamageRequest_RollInApp{RollInApp: true}
}

func typedDamage(sum int32) func(*playv1.RollDamageRequest) {
	return func(r *playv1.RollDamageRequest) { r.Roll = &playv1.RollDamageRequest_TypedSum{TypedSum: sum} }
}

func (a *armed) damage(t *testing.T, u *user, e *playv1.Encounter, pendingID string, roll func(*playv1.RollDamageRequest)) (*playv1.RollDamageResponse, error) {
	t.Helper()
	req := &playv1.RollDamageRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: pendingID, IdempotencyKey: newKey()}
	roll(req)
	res, err := u.combat.RollDamage(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (a *armed) mustDamage(t *testing.T, u *user, e *playv1.Encounter, pendingID string, roll func(*playv1.RollDamageRequest)) *playv1.RollDamageResponse {
	t.Helper()
	res, err := a.damage(t, u, e, pendingID, roll)
	if err != nil {
		t.Fatalf("RollDamage() error = %v", err)
	}
	return res
}

// settle calls ApplyPendingDamage or DiscardPendingDamage as u.
func (a *armed) settle(t *testing.T, u *user, e *playv1.Encounter, pendingID string, apply bool) (*playv1.PendingDamage, error) {
	t.Helper()
	if apply {
		res, err := u.combat.ApplyPendingDamage(t.Context(), connect.NewRequest(&playv1.ApplyPendingDamageRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: pendingID, IdempotencyKey: newKey(),
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetPendingDamage(), nil
	}
	res, err := u.combat.DiscardPendingDamage(t.Context(), connect.NewRequest(&playv1.DiscardPendingDamageRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: pendingID, IdempotencyKey: newKey(),
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetPendingDamage(), nil
}

// action calls TakeAction as u.
func (a *armed) action(t *testing.T, u *user, e *playv1.Encounter, label, key string) (*playv1.Encounter, error) {
	t.Helper()
	res, err := u.combat.TakeAction(t.Context(), connect.NewRequest(&playv1.TakeActionRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, label), ActionKey: key, IdempotencyKey: newKey(),
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetEncounter(), nil
}

// options calls GetTurnOptions as u.
func (a *armed) options(t *testing.T, u *user, e *playv1.Encounter, label string) (*playv1.GetTurnOptionsResponse, error) {
	t.Helper()
	res, err := u.combat.GetTurnOptions(t.Context(), connect.NewRequest(&playv1.GetTurnOptionsRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, label),
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (a *armed) mustOptions(t *testing.T, u *user, e *playv1.Encounter, label string) *playv1.GetTurnOptionsResponse {
	t.Helper()
	res, err := a.options(t, u, e, label)
	if err != nil {
		t.Fatalf("GetTurnOptions(%s) error = %v", label, err)
	}
	return res
}

// endTurn ends the turn of the combatant on turn as u, with the flag that
// drops a pending damage.
func (a *armed) endTurn(t *testing.T, u *user, e *playv1.Encounter, discard bool) (*playv1.Encounter, error) {
	t.Helper()
	cur := a.get(t, a.master).GetCurrentCombatantId()
	res, err := u.combat.EndTurn(t.Context(), connect.NewRequest(&playv1.EndTurnRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur, DiscardPendingDamage: discard,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetEncounter(), nil
}

func (a *armed) mustEndTurn(t *testing.T, u *user, e *playv1.Encounter) *playv1.Encounter {
	t.Helper()
	out, err := a.endTurn(t, u, e, false)
	if err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	return out
}

// log calls ListCombatLog as u.
func (a *armed) log(t *testing.T, u *user, e *playv1.Encounter) *playv1.ListCombatLogResponse {
	t.Helper()
	res, err := u.combat.ListCombatLog(t.Context(), connect.NewRequest(&playv1.ListCombatLogRequest{CampaignId: a.campaignID, EncounterId: e.GetId()}))
	if err != nil {
		t.Fatalf("ListCombatLog() error = %v", err)
	}
	return res.Msg
}

// undo calls UndoLastAction as u.
func (a *armed) undo(t *testing.T, u *user, e *playv1.Encounter, expected string) error {
	t.Helper()
	_, err := u.combat.UndoLastAction(t.Context(), connect.NewRequest(&playv1.UndoLastActionRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), ExpectedEventId: expected, IdempotencyKey: newKey(),
	}))
	return err
}

// adjustHP calls AdjustCombatantHitPoints as the master.
func (a *armed) adjustHP(t *testing.T, e *playv1.Encounter, label string, edit func(*playv1.AdjustCombatantHitPointsRequest)) (*playv1.Encounter, error) {
	t.Helper()
	req := &playv1.AdjustCombatantHitPointsRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, label), IdempotencyKey: newKey()}
	edit(req)
	res, err := a.master.combat.AdjustCombatantHitPoints(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetEncounter(), nil
}

func damageHP(n int32) func(*playv1.AdjustCombatantHitPointsRequest) {
	return func(r *playv1.AdjustCombatantHitPointsRequest) {
		r.Change = &playv1.AdjustCombatantHitPointsRequest_Damage{Damage: n}
	}
}

func healHP(n int32) func(*playv1.AdjustCombatantHitPointsRequest) {
	return func(r *playv1.AdjustCombatantHitPointsRequest) {
		r.Change = &playv1.AdjustCombatantHitPointsRequest_Heal{Heal: n}
	}
}

// vitals reads a player character's vitals as the master.
func (a *armed) vitals(t *testing.T, c *charactersv1.Character) *playv1.CharacterVitals {
	t.Helper()
	for _, v := range a.master.liveSession(t, a.campaignID).GetVitals() {
		if v.GetCharacterId() == c.GetId() {
			return v
		}
	}
	t.Fatalf("no vitals for %s", c.GetName())
	return nil
}

// hp reads an NPC's hit points as the master sees them.
func (a *armed) hp(t *testing.T, label string) (current, temp int32, defeated bool) {
	t.Helper()
	c := byLabel(t, a.get(t, a.master), label)
	return c.GetHitPointsCurrent(), c.GetHitPointsTemporary(), c.GetDefeated()
}

func attackOption(o *playv1.GetTurnOptionsResponse, key string) *rulesv1.AttackOption {
	for _, a := range o.GetOptions().GetAttacks() {
		if a.GetAttack().GetKey() == key {
			return a
		}
	}
	return nil
}

func standardOption(o *playv1.GetTurnOptionsResponse, key string) *rulesv1.ActionOption {
	for _, a := range o.GetOptions().GetStandardActions() {
		if a.GetAction().GetKey() == key {
			return a
		}
	}
	return nil
}

func targetOf(o *playv1.GetTurnOptionsResponse, attackKey, label string) *playv1.TargetInReach {
	for _, at := range o.GetAttackTargets() {
		if at.GetAttackKey() != attackKey {
			continue
		}
		for _, tg := range at.GetTargets() {
			if tg.GetLabel() == label {
				return tg
			}
		}
	}
	return nil
}

// wantBlockedBy fails unless err is the failed_precondition with this reason.
func wantBlockedBy(t *testing.T, call string, err error, reason playv1.EncounterBlockedReason) *playv1.EncounterBlocked {
	t.Helper()
	if err == nil {
		t.Fatalf("%s succeeded, want %v", call, reason)
	}
	return wantEncounterBlocked(t, err, reason)
}

var (
	blockedActionUsed     = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED
	blockedOutOfReach     = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_OUT_OF_REACH
	blockedWrongDice      = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WRONG_DICE_MODE
	blockedPendingDamage  = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_PENDING_DAMAGE
	blockedNothingToUndo  = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOTHING_TO_UNDO
	blockedDamageResolved = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_RESOLVED
	blockedNotRolled      = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_NOT_ROLLED
	blockedAlreadyRolled  = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DAMAGE_ALREADY_ROLLED
	blockedTargetDefeated = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TARGET_DEFEATED
	blockedNotActive      = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE
	blockedDown           = playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_COMBATANT_DOWN
)

func TestMR014_TurnOptionsFollowTheEconomy(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	// Before the turns run, everything is disabled and says why.
	setup := a.start(t, plan{
		npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{3},
		players: map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1}, reveal: []string{"Goblin"},
		at:    map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
		setup: true,
	})
	o := a.mustOptions(t, a.caio, setup, "Toren")
	if o.GetYourTurn() || attackOption(o, battleaxe).GetEnabled() || attackOption(o, battleaxe).GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_COMBAT_NOT_ACTIVE {
		t.Fatalf("options in SETUP = %v, want everything disabled by COMBAT_NOT_ACTIVE", o)
	}
	_, err := a.attack(t, a.caio, setup, "Toren", battleaxe, "Goblin", inAppRoll)
	wantBlockedBy(t, "an attack before the combat runs", err, blockedNotActive)
	if _, err := a.master.combat.BeginCombat(t.Context(), connect.NewRequest(&playv1.BeginCombatRequest{CampaignId: a.campaignID, EncounterId: setup.GetId(), IdempotencyKey: newKey()})); err != nil {
		t.Fatalf("BeginCombat() error = %v", err)
	}
	e := a.get(t, a.master)

	// Toren is on turn: the attack, the standard actions and 30 ft (6 squares)
	// of movement are open. Targets come with the distance (RN-21): the
	// goblin is one square away, Pensantus seven.
	o = a.mustOptions(t, a.caio, e, "Toren")
	if !o.GetYourTurn() || !attackOption(o, battleaxe).GetEnabled() || !o.GetOptions().GetEconomy().GetAction().GetAvailable() {
		t.Fatalf("Toren's options = %v, want his turn, the Machado de batalha enabled and the action available", o)
	}
	if mv := o.GetOptions().GetEconomy().GetMovement(); mv.GetSpeedFt() != 30 || mv.GetLeftFt() != 30 {
		t.Errorf("movement = %v, want 30 ft of 30", mv)
	}
	if tg := targetOf(o, battleaxe, "Goblin"); tg == nil || tg.GetDistanceFt() != 5 || tg.TooFar || tg.GetState() != playv1.CombatantState_COMBATANT_STATE_UNHURT {
		t.Errorf("the goblin as a target = %v, want 5 ft, in reach, Ileso", tg)
	}
	if tg := targetOf(o, battleaxe, "Pensantus"); tg == nil || tg.GetDistanceFt() != 35 || !tg.GetTooFar() {
		t.Errorf("Pensantus as a target = %v, want 35 ft and too far for a melee weapon", tg)
	}
	if targetOf(o, battleaxe, "Toren") != nil {
		t.Error("an attacker is never its own target")
	}
	// The Machado de batalha reaches 5 ft and Pensantus is 35 ft away: the
	// server refuses a player and says by how much (30 ft).
	_, err = a.attack(t, a.caio, e, "Toren", battleaxe, "Pensantus", inAppRoll)
	if b := wantBlockedBy(t, "an attack out of reach", err, blockedOutOfReach); b.GetMissingFt() != 30 {
		t.Errorf("missing_ft = %d, want 30", b.GetMissingFt())
	}

	// Pensantus is off turn: all disabled by NOT_YOUR_TURN. Toren's player
	// cannot read her options.
	po := a.mustOptions(t, a.ana, e, "Pensantus")
	for _, dis := range []interface {
		GetEnabled() bool
		GetReason() *rulesv1.DisabledReason
	}{attackOption(po, fireBolt), standardOption(po, "standard:dash")} {
		if dis.GetEnabled() || dis.GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_NOT_YOUR_TURN {
			t.Errorf("Pensantus off turn: %v, want disabled by NOT_YOUR_TURN", dis)
		}
	}
	if _, err := a.options(t, a.caio, e, "Pensantus"); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("another player's options: %v, want permission_denied", err)
	}
	_, err = a.action(t, a.ana, e, "Pensantus", "standard:dash")
	wantEncounterBlocked(t, err, reasonNotTurn)

	// The attack spends the action: the attack and the standard actions say so,
	// the movement is still there, and the damage waits to be rolled.
	a.h.roller.queue(15, 2)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	if hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT || hit.GetPendingDamage() == nil {
		t.Fatalf("attack = %v, want a hit with a pending damage", hit)
	}
	o = a.mustOptions(t, a.caio, a.get(t, a.caio), "Toren")
	if got := attackOption(o, battleaxe); got.GetEnabled() || got.GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ACTION_USED {
		t.Errorf("the attack after attacking = %v, want disabled by ACTION_USED", got)
	}
	if got := standardOption(o, "standard:dash"); got.GetEnabled() {
		t.Errorf("Disparada after attacking = %v, want disabled (the action is used)", got)
	}
	if !o.GetOptions().GetEconomy().GetAction().GetUsed() || o.GetOptions().GetEconomy().GetMovement().GetLeftFt() != 30 {
		t.Errorf("economy = %v, want the action used and the movement untouched", o.GetOptions().GetEconomy())
	}
	if len(o.GetPendingDamages()) != 1 || o.GetPendingDamages()[0].GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL {
		t.Errorf("pending damages = %v, want the one still to roll", o.GetPendingDamages())
	}
	_, err = a.attack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	wantBlockedBy(t, "a second attack", err, blockedActionUsed)

	_, err = a.action(t, a.caio, e, "Toren", "standard:dodge")
	wantBlockedBy(t, "Esquivar after attacking", err, blockedActionUsed)
	a.mustDamage(t, a.caio, e, hit.GetPendingDamage().GetId(), inAppDamage)

	// Attack and Cast a Spell are not standard actions to take; unknown keys
	// are refused.
	for _, key := range []string{"standard:attack", "standard:cast-a-spell", "standard:fly"} {
		if _, err := a.action(t, a.caio, e, "Toren", key); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("TakeAction(%s) = %v, want invalid_argument", key, err)
		}
	}

	// Pensantus takes the Dash action: her 25 ft become 50, and her action is used.
	a.mustEndTurn(t, a.caio, e)
	e = a.get(t, a.ana)
	if _, err := a.action(t, a.ana, e, "Pensantus", "standard:dash"); err != nil {
		t.Fatalf("TakeAction(dash) error = %v", err)
	}
	po = a.mustOptions(t, a.ana, e, "Pensantus")
	if mv := po.GetOptions().GetEconomy().GetMovement(); mv.GetSpeedFt() != 50 || mv.GetLeftFt() != 50 {
		t.Errorf("movement after Disparada = %v, want 50 ft (25 doubled)", mv)
	}
	if got := attackOption(po, fireBolt); got.GetEnabled() || got.GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_ACTION_USED {
		t.Errorf("Raio de Fogo after Disparada = %v, want disabled by ACTION_USED", got)
	}

	// A new turn resets the economy: the round goes round, and Toren's action
	// is back.
	a.mustEndTurn(t, a.ana, e)    // Brisa
	a.mustEndTurn(t, a.bia, e)    // the goblin
	a.mustEndTurn(t, a.master, e) // Toren, round 2
	o = a.mustOptions(t, a.caio, a.get(t, a.caio), "Toren")
	if !attackOption(o, battleaxe).GetEnabled() || o.GetOptions().GetEconomy().GetAction().GetUsed() {
		t.Errorf("Toren's options in round 2 = %v, want the action back", o)
	}
}

func TestMR012_DamageToAnNPCIsAppliedAndDefeatsIt(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	// The goblin has 7 hit points and the master gives it 3 temporary ones.
	if _, err := a.adjustHP(t, e, "Goblin", func(r *playv1.AdjustCombatantHitPointsRequest) { r.HitPointsTemporary = proto.Int32(3) }); err != nil {
		t.Fatalf("AdjustCombatantHitPoints(temp) error = %v", err)
	}

	// 1d8 (2) + 3 = 5 slashing: the temporary hit points take 3, the hit
	// points the other 2, applied at once.
	a.h.roller.queue(15, 2)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	if d := hit.GetRoll().GetD20(); d.GetTotal() != 20 || d.GetModifier() != 5 || len(d.GetFaces()) != 1 || d.GetFaces()[0] != 15 {
		t.Fatalf("attack roll = %v, want 1d20 (15) + 5 = 20", d)
	}
	dmg := a.mustDamage(t, a.caio, e, hit.GetPendingDamage().GetId(), inAppDamage).GetPendingDamage()
	if dmg.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED || dmg.GetAmount() != 5 || dmg.GetDamageTypeKey() != "damage-type:slashing" || dmg.GetDamageTypePt() != "cortante" {
		t.Fatalf("damage = %v, want 5 of cortante, applied at once to an NPC", dmg)
	}
	if r := dmg.GetRoll(); r.GetDiceCount() != 1 || r.GetDiceSides() != 8 || r.GetModifier() != 3 || r.GetTotal() != 5 || r.GetFaces()[0] != 2 {
		t.Errorf("damage roll = %v, want 1d8 (2) + 3 = 5", r)
	}
	if cur, temp, defeated := a.hp(t, "Goblin"); cur != 5 || temp != 0 || defeated {
		t.Fatalf("goblin = %d PV, %d temporários, defeated %v; want 5 PV, 0 temporários after 3 temporary and 2 real", cur, temp, defeated)
	}
	// The player sees a word, never a number (RN-20).
	seen := byLabel(t, a.get(t, a.ana), "Goblin")
	if seen.GetState() != playv1.CombatantState_COMBATANT_STATE_HURT || seen.HitPointsCurrent != nil {
		t.Errorf("goblin for a player = %v, want Ferido and no hit points", seen)
	}

	// A second blow in the next round: 1d8 (8) + 3 = 11 takes it to 0: it is
	// defeated, the answer says so, and the turns skip it.
	a.mustEndTurn(t, a.caio, e)
	a.mustEndTurn(t, a.ana, e)
	a.mustEndTurn(t, a.bia, e)
	a.mustEndTurn(t, a.master, e)
	a.h.roller.queue(15, 8)
	hit = a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	dmg = a.mustDamage(t, a.caio, e, hit.GetPendingDamage().GetId(), inAppDamage).GetPendingDamage()
	if !dmg.GetTargetDefeated() || dmg.GetAmount() != 11 {
		t.Fatalf("damage = %v, want 11 and the goblin defeated", dmg)
	}
	if cur, _, defeated := a.hp(t, "Goblin"); cur != 0 || !defeated {
		t.Fatalf("goblin = %d PV, defeated %v; want 0 PV and defeated", cur, defeated)
	}
	if got := byLabel(t, a.get(t, a.ana), "Goblin"); got.GetState() != playv1.CombatantState_COMBATANT_STATE_DEFEATED {
		t.Errorf("goblin for a player = %v, want Derrotado", got.GetState())
	}
	_, err := a.attack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	wantBlockedBy(t, "attacking a defeated goblin", err, blockedTargetDefeated)
	a.mustEndTurn(t, a.caio, e)
	a.mustEndTurn(t, a.ana, e)
	after := a.mustEndTurn(t, a.bia, e) // the goblin is defeated: the turn skips it
	if cur := byLabel(t, a.get(t, a.master), "Toren"); after.GetRound() != 3 || a.get(t, a.master).GetCurrentCombatantId() != cur.GetId() {
		t.Errorf("after Brisa: round %d, on turn %s; want round 3 and Toren (the defeated goblin skipped)", after.GetRound(), a.get(t, a.master).GetCurrentCombatantId())
	}

	// The master's hand: healing a defeated NPC brings it back into the order;
	// the arithmetic is the vitals' (damage takes temporary hit points first,
	// healing stops at the maximum).
	back, err := a.adjustHP(t, e, "Goblin", healHP(3))
	if err != nil {
		t.Fatalf("AdjustCombatantHitPoints(heal) error = %v", err)
	}
	if g := byLabel(t, back, "Goblin"); g.GetHitPointsCurrent() != 3 || g.GetDefeated() {
		t.Errorf("goblin after the heal = %v, want 3 PV and back in the fight", g)
	}
	back, _ = a.adjustHP(t, e, "Goblin", healHP(100))
	if g := byLabel(t, back, "Goblin"); g.GetHitPointsCurrent() != 7 {
		t.Errorf("goblin after healing 100 = %v, want the maximum, 7", g.GetHitPointsCurrent())
	}
	back, _ = a.adjustHP(t, e, "Goblin", func(r *playv1.AdjustCombatantHitPointsRequest) {
		r.Change = &playv1.AdjustCombatantHitPointsRequest_HitPoints{HitPoints: 0}
	})
	if g := byLabel(t, back, "Goblin"); g.GetHitPointsCurrent() != 0 || !g.GetDefeated() {
		t.Errorf("goblin set to 0 = %v, want defeated", g)
	}
	for name, edit := range map[string]func(*playv1.AdjustCombatantHitPointsRequest){
		"negative damage": damageHP(-1), "huge heal": healHP(10000), "nothing": func(*playv1.AdjustCombatantHitPointsRequest) {},
		"above the maximum": func(r *playv1.AdjustCombatantHitPointsRequest) {
			r.Change = &playv1.AdjustCombatantHitPointsRequest_HitPoints{HitPoints: 8}
		},
		"temp too big": func(r *playv1.AdjustCombatantHitPointsRequest) { r.HitPointsTemporary = proto.Int32(1000) },
	} {
		if _, err := a.adjustHP(t, e, "Goblin", edit); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("AdjustCombatantHitPoints(%s) = %v, want invalid_argument", name, err)
		}
	}
	if _, err := a.adjustHP(t, e, "Toren", damageHP(1)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("AdjustCombatantHitPoints(a player's character) = %v, want invalid_argument (use the vitals)", err)
	}
}

func TestRN02_DamageToAPlayerWaitsForTheMaster(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	// The Capitão Goblin plays first.
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.capitao.GetId(), Hidden: new(false)}},
		npcRolls: []int{19},
		players:  map[string]int32{"Toren": 12, "Pensantus": 8, "Brisa": 1},
		at:       map[string][2]int32{"Capitão Goblin": {6, 3}, "Toren": {5, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	toren := a.vitals(t, a.toren)

	// 1d20 (15) + 4 = 19 hits Toren (CA 12); 1d6 (3) + 2 = 5 piercing waits.
	a.h.roller.queue(15, 3)
	hit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Toren", inAppRoll)
	if hit.GetPendingDamage() == nil {
		t.Fatalf("attack = %v, want a hit", hit)
	}
	pending := a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage).GetPendingDamage()
	if pending.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED || pending.GetAmount() != 5 {
		t.Fatalf("damage = %v, want 5, rolled and waiting for the master", pending)
	}
	if got := a.vitals(t, a.toren); got.GetHitPointsCurrent() != toren.GetHitPointsCurrent() {
		t.Fatalf("Toren's hit points = %d before the master applies it, want %d", got.GetHitPointsCurrent(), toren.GetHitPointsCurrent())
	}

	// The master passes the turn only knowing it: PENDING_DAMAGE, unless he says so.
	_, err := a.endTurn(t, a.master, e, false)
	wantBlockedBy(t, "EndTurn with a damage waiting", err, blockedPendingDamage)
	// Only the master applies it.
	_, err = a.settle(t, a.caio, e, pending.GetId(), true)
	wantCode(t, "ApplyPendingDamage as a player", err, connect.CodePermissionDenied)
	applied, err := a.settle(t, a.master, e, pending.GetId(), true)
	if err != nil || applied.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED {
		t.Fatalf("ApplyPendingDamage() = %v, %v; want applied", applied, err)
	}
	if got := a.vitals(t, a.toren); got.GetHitPointsCurrent() != toren.GetHitPointsCurrent()-5 {
		t.Fatalf("Toren's hit points = %d, want %d (5 less, through the vitals)", got.GetHitPointsCurrent(), toren.GetHitPointsCurrent()-5)
	}
	_, err = a.settle(t, a.master, e, pending.GetId(), true)
	wantBlockedBy(t, "applying twice", err, blockedDamageResolved)
	_, err = a.settle(t, a.master, e, pending.GetId(), false)
	wantBlockedBy(t, "discarding an applied damage", err, blockedDamageResolved)
	_, err = a.damage(t, a.master, e, pending.GetId(), inAppDamage)
	wantBlockedBy(t, "rolling an applied damage", err, blockedDamageResolved)

	// A second hit, this time on Pensantus: it cannot be applied before it is
	// rolled, nor rolled twice; "Não aplicar" drops it and the hit points stay.
	pens := a.vitals(t, a.pens)
	a.h.roller.queue(15, 6)
	hit = a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", inAppRoll)
	id := hit.GetPendingDamage().GetId()
	_, err = a.settle(t, a.master, e, id, true)
	wantBlockedBy(t, "applying before rolling", err, blockedNotRolled)
	a.mustDamage(t, a.master, e, id, inAppDamage)
	_, err = a.damage(t, a.master, e, id, inAppDamage)
	wantBlockedBy(t, "rolling twice", err, blockedAlreadyRolled)
	discarded, err := a.settle(t, a.master, e, id, false)
	if err != nil || discarded.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_DISCARDED {
		t.Fatalf("DiscardPendingDamage() = %v, %v; want discarded", discarded, err)
	}
	if got := a.vitals(t, a.pens); got.GetHitPointsCurrent() != pens.GetHitPointsCurrent() {
		t.Fatalf("Pensantus's hit points = %d after discarding, want %d", got.GetHitPointsCurrent(), pens.GetHitPointsCurrent())
	}

	// A third one is left open: the master passes the turn anyway, which drops it.
	hit = a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Brisa", inAppRoll)
	left := hit.GetPendingDamage().GetId()
	if _, err := a.endTurn(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn(discard_pending_damage) error = %v", err)
	}
	if _, err := a.settle(t, a.master, e, left, true); err == nil {
		t.Error("the damage left open by a turn passed anyway can still be applied, want it discarded")
	}

	// A player cannot pass their turn over their own open damage, flag or not.
	a.h.roller.queue(15)
	hit = a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Capitão Goblin", inAppRoll)
	if hit.GetPendingDamage() == nil {
		t.Fatalf("Toren's attack = %v, want a hit on the Capitão (CA 18)", hit)
	}
	_, err = a.endTurn(t, a.caio, e, true)
	wantBlockedBy(t, "a player's EndTurn with a damage to roll", err, blockedPendingDamage)
	a.h.roller.queue(4)
	a.mustDamage(t, a.caio, e, hit.GetPendingDamage().GetId(), inAppDamage)
	a.mustEndTurn(t, a.caio, e)

	// At 0 hit points a character is down: everyone sees the word, nobody
	// acts for them, and the numbers stay the master's.
	if _, err := a.master.play.AdjustCharacterVitals(t.Context(), connect.NewRequest(&playv1.AdjustCharacterVitalsRequest{
		CampaignId: a.campaignID, CharacterId: a.pens.GetId(), IdempotencyKey: newKey(), HitPointsCurrent: proto.Int32(3),
	})); err != nil {
		t.Fatalf("AdjustCharacterVitals() error = %v", err)
	}
	a.mustEndTurn(t, a.ana, e) // Pensantus
	a.mustEndTurn(t, a.bia, e) // Brisa -> the Capitão, round 2
	a.h.roller.queue(15, 6)    // 1d6 (6) + 2 = 8, more than the 3 she has
	hit = a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", inAppRoll)
	id = hit.GetPendingDamage().GetId()
	a.mustDamage(t, a.master, e, id, inAppDamage)
	if _, err := a.settle(t, a.master, e, id, true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	if got := a.vitals(t, a.pens); got.GetHitPointsCurrent() != 0 {
		t.Fatalf("Pensantus's hit points = %d, want 0 (never below)", got.GetHitPointsCurrent())
	}
	for _, p := range []*user{a.ana, a.caio} {
		seen := byLabel(t, a.get(t, p), "Pensantus")
		if seen.GetState() != playv1.CombatantState_COMBATANT_STATE_DOWN || seen.HitPointsCurrent != nil {
			t.Errorf("Pensantus for a player = %v, want Caída and no hit points", seen)
		}
	}
	// On her turn, the options say she is down, and an attack is refused.
	a.mustEndTurn(t, a.master, e) // Toren
	a.mustEndTurn(t, a.caio, e)   // Pensantus, down
	po := a.mustOptions(t, a.ana, e, "Pensantus")
	if got := attackOption(po, fireBolt); got.GetEnabled() || got.GetReason().GetCode() != rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_COMBATANT_DOWN {
		t.Errorf("Pensantus down = %v, want disabled by COMBATANT_DOWN", got)
	}
	_, err = a.attack(t, a.ana, e, "Pensantus", fireBolt, "Capitão Goblin", inAppRoll)
	wantBlockedBy(t, "a downed character's attack", err, blockedDown)
}

// json is a message as the player's browser receives it.
func asJSON(t *testing.T, m proto.Message) string {
	t.Helper()
	raw, err := protojson.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return string(raw)
}

func TestRN20_PlayersNeverReceiveCAOrHiddenLogEntries(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	// Two hidden goblins and the Capitão, who is in plain sight. Goblin 1
	// plays first, then Toren, Pensantus, the Capitão, Brisa and Goblin 2.
	e := a.start(t, plan{
		npcs: []*playv1.Participant{
			{CharacterId: a.capitao.GetId(), Hidden: new(false)},
			{CharacterId: a.goblin.GetId(), Count: 2}, // hidden, the default
		},
		npcRolls: []int{5, 20, 4}, // Capitão, Goblin 1, Goblin 2
		players:  map[string]int32{"Toren": 17, "Pensantus": 10, "Brisa": 1},
		at:       map[string][2]int32{"Toren": {3, 3}, "Capitão Goblin": {4, 3}, "Goblin 1": {2, 3}, "Goblin 2": {9, 9}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	g1, g2 := a.id(t, "Goblin 1"), a.id(t, "Goblin 2")
	if cur := a.get(t, a.master).GetCurrentCombatantId(); cur != g1 {
		t.Fatalf("on turn = %s, want the hidden Goblin 1", cur)
	}

	// The hidden Goblin 1 hits Toren; the master applies 1d6 (4) + 2 = 6.
	a.h.roller.queue(15, 4)
	hit := a.mustAttack(t, a.master, e, "Goblin 1", sword, "Toren", inAppRoll)
	id := hit.GetPendingDamage().GetId()
	a.mustDamage(t, a.master, e, id, inAppDamage)
	if _, err := a.settle(t, a.master, e, id, true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	a.mustEndTurn(t, a.master, e)

	// Toren hits the Capitão (CA 18): 1d20 (15) + 5 = 20; 1d8 (6) + 3 = 9.
	a.h.roller.queue(15, 6)
	torenHit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Capitão Goblin", inAppRoll)
	torenDamage := a.mustDamage(t, a.caio, e, torenHit.GetPendingDamage().GetId(), inAppDamage)
	a.mustEndTurn(t, a.caio, e)
	a.mustEndTurn(t, a.ana, e) // Pensantus; the Capitão is next

	// The Capitão hits Pensantus: 1d20 (16) + 4 = 20; 1d6 (5) + 2 = 7, applied by the master.
	a.h.roller.queue(16, 5)
	capHit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Pensantus", inAppRoll)
	capID := capHit.GetPendingDamage().GetId()
	a.mustDamage(t, a.master, e, capID, inAppDamage)
	if _, err := a.settle(t, a.master, e, capID, true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}

	// What a player's browser gets, of everything this slice returns: no armor
	// class, no NPC hit points, no hidden combatant.
	seen := map[string][]string{ // who -> the messages as JSON
		"Toren's player": {
			asJSON(t, torenHit), asJSON(t, torenDamage), asJSON(t, a.mustOptions(t, a.caio, a.get(t, a.caio), "Toren")),
			asJSON(t, a.log(t, a.caio, e)), asJSON(t, a.get(t, a.caio)),
		},
		"Pensantus's player": {
			asJSON(t, a.mustOptions(t, a.ana, a.get(t, a.ana), "Pensantus")), asJSON(t, a.log(t, a.ana, e)), asJSON(t, a.get(t, a.ana)),
		},
	}
	for who, msgs := range seen {
		for _, text := range msgs {
			for _, banned := range []string{"armor", "Armor", "hitPointsCurrent", "hitPointsMax", g1, g2, "Goblin 1", "Goblin 2", a.goblin.GetId(), a.capitao.GetId()} {
				if strings.Contains(text, banned) {
					t.Errorf("a message for %s has %q: %s", who, banned, text)
				}
			}
		}
	}

	// The master, on the other hand, reads each combatant's armor class in the
	// order list, and the class an attack was compared with in its roll.
	masterView := a.get(t, a.master)
	for label, want := range map[string]int32{"Capitão Goblin": 18} {
		if got := byLabel(t, masterView, label).GetArmorClass(); got != want {
			t.Errorf("the master's %s armor class = %d, want %d", label, got, want)
		}
	}
	pens := byLabel(t, masterView, "Pensantus")
	if pens.ArmorClass == nil || pens.GetArmorClass() <= 0 {
		t.Errorf("the master's Pensantus = %v, want an armor class", pens)
	}
	if got := capHit.GetRoll().GetTargetArmorClass(); got != pens.GetArmorClass() {
		t.Errorf("the master's roll target armor class = %d, want Pensantus's %d", got, pens.GetArmorClass())
	}

	// The master's log has the hidden goblin's attack, marked as his alone;
	// the players' logs do not have it at all.
	var masterLines, torenLines, pensLines []string
	for _, r := range a.log(t, a.master, e).GetRounds() {
		for _, en := range r.GetEntries() {
			masterLines = append(masterLines, en.GetActorLabel()+">"+en.GetTargetLabel())
			if en.GetActorLabel() == "Goblin 1" && !en.GetHidden() {
				t.Errorf("the hidden goblin's entry for the master = %v, want hidden set", en)
			}
			if en.GetActorLabel() == "Goblin 1" && en.GetAttackRoll().GetTotal() != 19 {
				t.Errorf("the master's entry of an NPC's attack = %v, want its roll (1d20 (15) + 4 = 19)", en)
			}
		}
	}
	for _, r := range a.log(t, a.caio, e).GetRounds() {
		for _, en := range r.GetEntries() {
			torenLines = append(torenLines, en.GetActorLabel()+">"+en.GetTargetLabel())
			if en.GetActorLabel() == "Toren" && en.GetAttackRoll().GetTotal() != 20 {
				t.Errorf("Toren's own entry = %v, want his dice (1d20 (15) + 5 = 20)", en)
			}
			if en.GetActorLabel() == "Capitão Goblin" && (en.GetAttackRoll() != nil || en.GetDamage().GetRoll() != nil || en.HitPointsAfter != nil || en.GetDamage().GetAmount() != 7) {
				t.Errorf("a player's entry of the Capitão's attack = %v, want the outcome and the 7 damage, never his dice or hit points", en)
			}
		}
	}
	for _, r := range a.log(t, a.ana, e).GetRounds() {
		for _, en := range r.GetEntries() {
			pensLines = append(pensLines, en.GetActorLabel()+">"+en.GetTargetLabel())
			if en.GetActorLabel() == "Toren" && en.GetAttackRoll() != nil {
				t.Errorf("another player's dice leaked to Pensantus's player: %v", en)
			}
		}
	}
	if !slices.Contains(masterLines, "Goblin 1>Toren") || slices.Contains(torenLines, "Goblin 1>Toren") || slices.Contains(pensLines, "Goblin 1>Toren") {
		t.Errorf("logs: master %v, Toren's player %v, Pensantus's player %v; want the hidden goblin's line for the master only", masterLines, torenLines, pensLines)
	}
	if !slices.Contains(torenLines, "Toren>Capitão Goblin") || !slices.Contains(torenLines, "Capitão Goblin>Pensantus") {
		t.Errorf("Toren's player log = %v, want the visible attacks", torenLines)
	}

	// Revealing the goblin does not bring its old lines to the players: only
	// what happens from now on.
	if _, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: g1, IdempotencyKey: newKey(), Hidden: false,
	})); err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}
	after := a.log(t, a.caio, e)
	for _, r := range after.GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetActorLabel() == "Goblin 1" || en.GetTargetLabel() == "Goblin 1" {
				t.Errorf("a revealed goblin's old entry reached a player: %v", en)
			}
		}
	}
	if byLabel(t, a.get(t, a.caio), "Goblin 1") == nil {
		t.Error("the revealed goblin is not in the player's order")
	}
	// The master's log says he revealed it, and a player's does not.
	if lines := asJSON(t, a.log(t, a.master, e)); !strings.Contains(lines, "REVEAL_CHANGED") {
		t.Errorf("the master's log = %s, want the reveal", lines)
	}
	if strings.Contains(asJSON(t, after), "REVEAL_CHANGED") {
		t.Error("a player's log has the master's reveal")
	}
}

func TestRN18_PhysicalRollsAreTypedSums(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.capitao.GetId(), Hidden: new(false)}},
		npcRolls: []int{11},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		at:       map[string][2]int32{"Toren": {3, 3}, "Capitão Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	a.setPhysical(t, a.caio) // Toren's saved preference: real dice

	// A mode the master forced binds everybody: with "todos rolam no app" a
	// typed face is refused; with "todos rolam os próprios dados", the app's
	// roll is (RN-18).
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_APP)
	_, err := a.attack(t, a.caio, e, "Toren", battleaxe, "Capitão Goblin", d20(16))
	wantBlockedBy(t, "a typed face when everybody rolls in the app", err, blockedWrongDice)
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	_, err = a.attack(t, a.caio, e, "Toren", battleaxe, "Capitão Goblin", inAppRoll)
	wantBlockedBy(t, "the app's roll when everybody rolls real dice", err, blockedWrongDice)
	// With "cada jogador escolhe" the player chooses on each roll: the saved
	// preference only says what the screen offers.
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PLAYERS_CHOOSE)

	// The d20 is typed: 1 to 20 (a bad face is refused whatever the mode).
	for _, face := range []int32{0, 21, -3} {
		if _, err := a.attack(t, a.caio, e, "Toren", battleaxe, "Capitão Goblin", d20(face)); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("d20_face %d = %v, want invalid_argument", face, err)
		}
	}
	if _, err := a.attack(t, a.caio, e, "Toren", battleaxe, "Capitão Goblin", func(*playv1.RollAttackRequest) {}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("no roll = %v, want invalid_argument", err)
	}
	// 16 + 5 = 21 reaches the Capitão's CA 18: a hit, with the typed face.
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Capitão Goblin", d20(16))
	if d := hit.GetRoll().GetD20(); !d.GetPhysical() || d.GetTotal() != 21 || hit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_HIT {
		t.Fatalf("typed attack = %v, want 16 + 5 = 21, physical, a hit", hit.GetRoll())
	}
	id := hit.GetPendingDamage().GetId()
	if p := hit.GetPendingDamage(); p.GetDiceCount() != 1 || p.GetDiceSides() != 8 || p.GetBonus() != 3 || p.GetCritical() {
		t.Errorf("pending damage = %v, want 1d8 + 3, not critical", p)
	}

	// The damage is the typed sum of the dice, without the modifier: 1d8 is 1 to 8.
	for _, sum := range []int32{0, 9, -1} {
		if _, err := a.damage(t, a.caio, e, id, typedDamage(sum)); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("typed_sum %d for 1d8 = %v, want invalid_argument", sum, err)
		}
	}
	dmg := a.mustDamage(t, a.caio, e, id, typedDamage(6)).GetPendingDamage()
	if r := dmg.GetRoll(); dmg.GetAmount() != 9 || !r.GetPhysical() || r.GetTotal() != 9 || len(r.GetFaces()) != 0 || r.GetModifier() != 3 {
		t.Fatalf("typed damage = %v, want 6 + 3 = 9, physical, no faces", dmg)
	}
	if cur, _, _ := a.hp(t, "Capitão Goblin"); cur != 18 {
		t.Errorf("the Capitão has %d PV, want 18 (27 - 9)", cur)
	}

	// Pensantus's preference is the app, and she may still type her damage:
	// a natural 20 in the app is a critical hit, whose damage dice double
	// (1d10 -> 2d10) and whose modifier is added once (none).
	a.mustEndTurn(t, a.caio, e)
	a.h.roller.queue(20)
	crit := a.mustAttack(t, a.ana, e, "Pensantus", fireBolt, "Capitão Goblin", inAppRoll)
	if crit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_CRITICAL_HIT || !crit.GetPendingDamage().GetCritical() || crit.GetPendingDamage().GetDiceCount() != 2 {
		t.Fatalf("natural 20 = %v, want a critical hit and 2 dice", crit)
	}
	dmg = a.mustDamage(t, a.ana, e, crit.GetPendingDamage().GetId(), typedDamage(7)).GetPendingDamage()
	if r := dmg.GetRoll(); dmg.GetAmount() != 7 || !r.GetPhysical() || r.GetDiceCount() != 2 || r.GetDiceSides() != 10 || dmg.GetDamageTypeKey() != "damage-type:fire" {
		t.Fatalf("critical damage = %v, want 2d10 typed as 7 of fire", dmg)
	}

	// The master types an NPC's roll too (a face of 1 to 20), and the damage's sum.
	a.mustEndTurn(t, a.ana, e) // the Capitão
	if _, err := a.attack(t, a.master, e, "Capitão Goblin", sword, "Toren", d20(25)); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("the master's d20_face 25 = %v, want invalid_argument", err)
	}
	npcHit := a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Toren", d20(15))
	if !npcHit.GetRoll().GetD20().GetPhysical() || npcHit.GetRoll().GetD20().GetTotal() != 19 {
		t.Errorf("the master's typed roll = %v, want 15 + 4 = 19", npcHit.GetRoll())
	}
	typed := a.mustDamage(t, a.master, e, npcHit.GetPendingDamage().GetId(), typedDamage(1)).GetPendingDamage()
	if typed.GetAmount() != 3 { // 1 + 2
		t.Errorf("the master's typed damage = %v, want 1 + 2 = 3", typed)
	}
	if _, err := a.settle(t, a.master, e, typed.GetId(), true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}

	// A natural 20 typed by a player with real dice doubles the dice, and the
	// sum's range doubles with it: 2d8 is 2 to 16.
	a.mustEndTurn(t, a.master, e) // Brisa
	a.mustEndTurn(t, a.bia, e)    // Toren, round 2
	a.forceDice(t, campaignsv1.DiceMode_DICE_MODE_PHYSICAL)
	crit = a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Capitão Goblin", d20(20))
	cid := crit.GetPendingDamage().GetId()
	_, err = a.damage(t, a.caio, e, cid, inAppDamage)
	wantBlockedBy(t, "the app's damage when everybody rolls real dice", err, blockedWrongDice)
	for _, sum := range []int32{1, 17} {
		if _, err := a.damage(t, a.caio, e, cid, typedDamage(sum)); connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("typed_sum %d for 2d8 = %v, want invalid_argument", sum, err)
		}
	}
	dmg = a.mustDamage(t, a.caio, e, cid, typedDamage(16)).GetPendingDamage()
	if dmg.GetAmount() != 19 || dmg.GetRoll().GetDiceCount() != 2 { // 16 + 3
		t.Errorf("typed critical damage = %v, want 2d8 = 16 + 3 = 19", dmg)
	}
}

func TestCombatUndoRestoresTheLastAction(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)

	// Toren kills the goblin: 1d8 (8) + 3 = 11 against its 7 hit points.
	a.h.roller.queue(15, 8)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	a.mustDamage(t, a.caio, e, hit.GetPendingDamage().GetId(), inAppDamage)
	if _, _, defeated := a.hp(t, "Goblin"); !defeated {
		t.Fatal("the goblin should be defeated")
	}
	lastEntry := func() *playv1.CombatLogEntry {
		for _, r := range a.log(t, a.master, e).GetRounds() {
			for _, en := range r.GetEntries() {
				if en.GetUndoable() {
					return en
				}
			}
		}
		return nil
	}
	l := a.log(t, a.master, e)
	if en := lastEntry(); en == nil || en.GetKind() != playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK || en.GetDamage().GetAmount() != 11 || l.GetUndoableEventId() == "" {
		t.Fatalf("the entry to undo = %v (event %s), want Toren's attack with 11 damage", lastEntry(), l.GetUndoableEventId())
	}
	if a.log(t, a.caio, e).GetUndoableEventId() != "" {
		t.Error("a player is told which event can be undone")
	}

	// Two masters: the second one's screen is stale, and nothing changes.
	if err := a.undo(t, a.master, e, newKey()); connect.CodeOf(err) != connect.CodeAborted {
		t.Fatalf("UndoLastAction(a stale event) = %v, want aborted", err)
	}
	if err := a.undo(t, a.caio, e, l.GetUndoableEventId()); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("UndoLastAction as a player = %v, want permission_denied", err)
	}
	// One step back: the goblin is alive with its 7 hit points, and the damage
	// waits to be rolled again.
	if err := a.undo(t, a.master, e, l.GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	if cur, temp, defeated := a.hp(t, "Goblin"); cur != 7 || temp != 0 || defeated {
		t.Fatalf("goblin after the undo = %d PV, %d temp, defeated %v; want 7, 0, alive", cur, temp, defeated)
	}
	o := a.mustOptions(t, a.caio, a.get(t, a.caio), "Toren")
	if len(o.GetPendingDamages()) != 1 || o.GetPendingDamages()[0].GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL || o.GetPendingDamages()[0].GetRoll() != nil {
		t.Fatalf("pending damages after the undo = %v, want the damage to roll again", o.GetPendingDamages())
	}
	// Another step: the attack itself, the action is back and the hit is gone.
	l = a.log(t, a.master, e)
	if err := a.undo(t, a.master, e, l.GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v", err)
	}
	o = a.mustOptions(t, a.caio, a.get(t, a.caio), "Toren")
	if len(o.GetPendingDamages()) != 0 || !attackOption(o, battleaxe).GetEnabled() || o.GetOptions().GetEconomy().GetAction().GetUsed() {
		t.Fatalf("after undoing the attack: %v, want no pending damage and the action back", o)
	}
	if strings.Contains(asJSON(t, a.log(t, a.master, e)), "ATTACK") {
		t.Error("the undone attack is still in the master's log")
	}
	// Nothing else to undo: the combat's beginning is not an action.
	l = a.log(t, a.master, e)
	if l.GetUndoableEventId() != "" {
		t.Errorf("undoable event = %s, want none", l.GetUndoableEventId())
	}
	wantBlockedBy(t, "UndoLastAction with nothing to undo", a.undo(t, a.master, e, newKey()), blockedNothingToUndo)

	// The Dash action and the master's hand are undone the same way.
	if _, err := a.action(t, a.caio, e, "Toren", "standard:dash"); err != nil {
		t.Fatalf("TakeAction() error = %v", err)
	}
	if mv := a.mustOptions(t, a.caio, a.get(t, a.caio), "Toren").GetOptions().GetEconomy().GetMovement(); mv.GetLeftFt() != 60 {
		t.Fatalf("movement after Disparada = %v, want 60", mv)
	}
	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction(dash) error = %v", err)
	}
	o = a.mustOptions(t, a.caio, a.get(t, a.caio), "Toren")
	if mv := o.GetOptions().GetEconomy().GetMovement(); mv.GetLeftFt() != 30 || o.GetOptions().GetEconomy().GetAction().GetUsed() {
		t.Fatalf("after undoing Disparada: %v, want 30 ft and the action back", o.GetOptions().GetEconomy())
	}
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(3)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction(hit points) error = %v", err)
	}
	if cur, _, _ := a.hp(t, "Goblin"); cur != 7 {
		t.Errorf("goblin after undoing the master's damage = %d, want 7", cur)
	}

	// The goblin attacks Toren; the master applies it, then walks it all back:
	// the vitals first, then the damage roll, then the attack.
	a.mustEndTurn(t, a.caio, e)
	a.mustEndTurn(t, a.ana, e)
	a.mustEndTurn(t, a.bia, e) // the goblin
	toren := a.vitals(t, a.toren)
	a.h.roller.queue(15, 3)
	hit = a.mustAttack(t, a.master, e, "Goblin", sword, "Toren", inAppRoll)
	id := hit.GetPendingDamage().GetId()
	a.mustDamage(t, a.master, e, id, inAppDamage)
	if _, err := a.settle(t, a.master, e, id, true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	step := func(wantHP int32, wantStatus playv1.PendingDamageStatus) {
		t.Helper()
		if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
			t.Fatalf("UndoLastAction() error = %v", err)
		}
		if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != wantHP {
			t.Errorf("Toren's hit points = %d, want %d", got, wantHP)
		}
		pend := a.mustOptions(t, a.master, e, "Goblin").GetPendingDamages()
		if wantStatus == 0 {
			if len(pend) != 0 {
				t.Errorf("pending damages = %v, want none", pend)
			}
		} else if len(pend) != 1 || pend[0].GetStatus() != wantStatus {
			t.Errorf("pending damages = %v, want one in %v", pend, wantStatus)
		}
	}
	step(toren.GetHitPointsCurrent(), playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED)
	step(toren.GetHitPointsCurrent(), playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL)
	step(toren.GetHitPointsCurrent(), 0)
	if o := a.mustOptions(t, a.master, e, "Goblin"); o.GetOptions().GetEconomy().GetAction().GetUsed() {
		t.Error("the goblin's action is not back after undoing its attack")
	}

	// What a later change makes impossible to put back: a discarded damage is
	// undone, but nothing is undone past a turn that passed, or a correction of
	// the vitals made in between.
	a.h.roller.queue(15, 3)
	hit = a.mustAttack(t, a.master, e, "Goblin", sword, "Toren", inAppRoll)
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage)
	if _, err := a.settle(t, a.master, e, hit.GetPendingDamage().GetId(), false); err != nil {
		t.Fatalf("DiscardPendingDamage() error = %v", err)
	}
	step(toren.GetHitPointsCurrent(), playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED) // the discard is taken back
	if _, err := a.master.play.AdjustCharacterVitals(t.Context(), connect.NewRequest(&playv1.AdjustCharacterVitalsRequest{
		CampaignId: a.campaignID, CharacterId: a.toren.GetId(), IdempotencyKey: newKey(), HitPointsCurrent: proto.Int32(1),
	})); err != nil {
		t.Fatalf("AdjustCharacterVitals() error = %v", err)
	}
	wantBlockedBy(t, "UndoLastAction after a correction of the vitals", a.undo(t, a.master, e, newKey()), blockedNothingToUndo)
	if _, err := a.endTurn(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn() error = %v", err)
	}
	wantBlockedBy(t, "UndoLastAction after a turn passed", a.undo(t, a.master, e, newKey()), blockedNothingToUndo)
}

// line writes a log entry as one short sentence, to compare whole logs.
func line(e *playv1.CombatLogEntry) string {
	who := e.GetActorLabel()
	var s string
	switch e.GetKind() {
	case playv1.CombatLogKind_COMBAT_LOG_KIND_COMBAT_BEGUN:
		s = "o combate começa"
	case playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION:
		s = who + ": " + e.GetKeyNamePt()
	case playv1.CombatLogKind_COMBAT_LOG_KIND_TURN_PART_ENDED:
		s = who + " encerrou a parte"
	case playv1.CombatLogKind_COMBAT_LOG_KIND_MOVED:
		s = fmt.Sprintf("%s anda %d ft", who, e.GetDistanceFt())
	case playv1.CombatLogKind_COMBAT_LOG_KIND_REVEAL_CHANGED:
		s = fmt.Sprintf("o mestre mostra %s (escondido: %v)", e.GetTargetLabel(), e.GetNowHidden())
	case playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK:
		outcome := map[playv1.AttackOutcome]string{
			playv1.AttackOutcome_ATTACK_OUTCOME_HIT: "acertou", playv1.AttackOutcome_ATTACK_OUTCOME_CRITICAL_HIT: "crítico", playv1.AttackOutcome_ATTACK_OUTCOME_MISS: "errou",
		}[e.GetOutcome()]
		s = fmt.Sprintf("%s -> %s (%s): %s", who, e.GetTargetLabel(), e.GetKeyNamePt(), outcome)
		if d := e.GetDamage(); d != nil {
			status := map[playv1.PendingDamageStatus]string{
				playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_AWAITING_ROLL: "sem rolar", playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED: "pendente",
				playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED: "aplicado", playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_DISCARDED: "descartado",
			}[d.GetStatus()]
			s += fmt.Sprintf(", %d %s, %s", d.GetAmount(), d.GetDamageTypePt(), status)
			if d.GetTargetDefeated() {
				s += ", derrotado"
			}
		}
	}
	if e.GetHidden() {
		s += " [só o mestre vê]"
	}
	return s
}

// lines is a whole log: "R2 ..." for each entry, the latest first.
func lines(l *playv1.ListCombatLogResponse) []string {
	var out []string
	for _, r := range l.GetRounds() {
		for _, e := range r.GetEntries() {
			out = append(out, fmt.Sprintf("R%d %s", r.GetRound(), line(e)))
		}
	}
	return out
}

func wantLines(t *testing.T, who string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s's log:\n%s\nwant:\n%s", who, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestTimelineRound1And2Log replays the canonical fight (timeline.md) as far
// as this slice goes, with scripted rolls, and checks the combat log of the
// master and of a player. The spells (Sono, Mísseis Mágicos) and the death
// saves come with the next slice, so Pensantus's Sono is her turn passing
// and Goblin 1's sleep is his.
func TestTimelineRound1And2Log(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs: []*playv1.Participant{
			{CharacterId: a.capitao.GetId(), Hidden: new(false)},
			{CharacterId: a.goblin.GetId(), Count: 3}, // hidden until the master shows them
		},
		// The order: Brisa 19, Capitão 16, Pensantus 14, Goblin 1 12, Goblin 2 12, Goblin 3 9, Toren 7.
		npcRolls: []int{16, 12, 12, 9},
		players:  map[string]int32{"Brisa": 16, "Pensantus": 12, "Toren": 5},
		reveal:   []string{"Goblin 1", "Goblin 2"},
		at: map[string][2]int32{
			"Brisa": {9, 4}, "Capitão Goblin": {10, 5}, "Pensantus": {8, 2}, "Goblin 1": {6, 5}, "Goblin 2": {14, 2}, "Goblin 3": {12, 8}, "Toren": {5, 5},
		},
	})
	if got := strings.Join(labels(a.get(t, a.master)), ", "); got != "Brisa, Capitão Goblin, Pensantus, Goblin 1, Goblin 2, Goblin 3, Toren" {
		t.Fatalf("order = %s", got)
	}
	toren, brisa := a.vitals(t, a.toren).GetHitPointsCurrent(), a.vitals(t, a.bri).GetHitPointsCurrent()

	// attackAndApply is one attack by label: the d20 and the damage die are
	// queued, the damage is applied at once for an NPC, by the master for a
	// player's character.
	attackAndApply := func(u *user, attacker, key, target string, d20, die int) {
		t.Helper()
		a.h.roller.queue(d20, die)
		hit := a.mustAttack(t, u, e, attacker, key, target, inAppRoll)
		if hit.GetPendingDamage() == nil {
			t.Fatalf("%s's attack on %s = %v, want a hit", attacker, target, hit.GetRoll())
		}
		dmg := a.mustDamage(t, u, e, hit.GetPendingDamage().GetId(), inAppDamage).GetPendingDamage()
		if dmg.GetStatus() == playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED {
			if _, err := a.settle(t, a.master, e, dmg.GetId(), true); err != nil {
				t.Fatalf("ApplyPendingDamage() error = %v", err)
			}
		}
	}
	end := func(u *user) { t.Helper(); a.mustEndTurn(t, u, e) }

	// Rodada 1.
	if _, err := a.action(t, a.bia, e, "Brisa", "standard:hide"); err != nil { // 1. Brisa se esconde atrás da carroça
		t.Fatalf("TakeAction(hide) error = %v", err)
	}
	end(a.bia)
	attackAndApply(a.master, "Capitão Goblin", sword, "Toren", 15, 3) // 2. Capitão acerta o Toren: 5 de dano
	end(a.master)
	end(a.ana)                                                     // 3. Pensantus (Sono: a próxima fatia)
	end(a.master)                                                  // 4. Goblin 1 dorme; Goblin 2 tem o mesmo 12 e joga no mesmo turno conjunto
	attackAndApply(a.master, "Goblin 2", shortBow, "Brisa", 15, 3) // 5. Goblin 2 atira na Brisa: 5 de dano
	end(a.master)
	if _, err := a.action(t, a.master, e, "Goblin 3", "standard:hide"); err != nil { // 6. Goblin 3 espera escondido
		t.Fatalf("TakeAction(hide, Goblin 3) error = %v", err)
	}
	end(a.master)
	attackAndApply(a.caio, "Toren", battleaxe, "Goblin 1", 15, 6) // 7. Toren acerta o Goblin 1: 9, derrotado
	if _, _, defeated := a.hp(t, "Goblin 1"); !defeated {
		t.Error("Goblin 1 should be defeated (9 of 7)")
	}
	if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != toren-5 {
		t.Errorf("Toren = %d, want %d (5 damage)", got, toren-5)
	}
	if got := a.vitals(t, a.bri).GetHitPointsCurrent(); got != brisa-5 {
		t.Errorf("Brisa = %d, want %d (5 damage)", got, brisa-5)
	}
	end(a.caio)

	// Rodada 2.
	if cur := a.get(t, a.master); cur.GetRound() != 2 || cur.GetCurrentCombatantId() != a.id(t, "Brisa") {
		t.Fatalf("round %d, on turn %s; want round 2 and Brisa", cur.GetRound(), cur.GetCurrentCombatantId())
	}
	attackAndApply(a.bia, "Brisa", rapier, "Capitão Goblin", 15, 5) // 1. Brisa acerta o Capitão: 8 de dano
	end(a.bia)
	// 2. Capitão atira no Toren (Arco curto): 1d20 (15) + 4 = 19; 1d6 (3) + 2 = 5, e o
	// dano espera o mestre.
	a.h.roller.queue(15, 3)
	hit := a.mustAttack(t, a.master, e, "Capitão Goblin", shortBow, "Toren", inAppRoll)
	pending := a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage).GetPendingDamage()
	if pending.GetAmount() != 5 || pending.GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_ROLLED {
		t.Fatalf("the Capitão's damage = %v, want 5, waiting", pending)
	}
	// E6-11: this moment, before "Aplicar 5 de dano": the entry is there, pending,
	// and Toren has not lost the hit points.
	pendingLog := lines(a.log(t, a.master, e))
	if want := "R2 Capitão Goblin -> Toren (Arco curto): acertou, 5 perfurante, pendente"; pendingLog[0] != want {
		t.Errorf("the master's latest line = %q, want %q", pendingLog[0], want)
	}
	if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != toren-5 {
		t.Errorf("Toren = %d before the apply, want %d", got, toren-5)
	}
	if _, err := a.settle(t, a.master, e, pending.GetId(), true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != toren-10 {
		t.Errorf("Toren = %d after the apply, want %d", got, toren-10)
	}
	end(a.master)

	// 3. Pensantus anda 2 quadrados (3 m) e acerta o Goblin 2 com o Raio de Fogo. Her player's
	// log at this moment (E6-15) has no word of Goblin 3.
	e = a.get(t, a.ana)
	if _, err := a.ana.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Pensantus"), IdempotencyKey: newKey(), Col: 10, Row: 2,
	})); err != nil {
		t.Fatalf("MoveCombatant() error = %v", err)
	}
	wantLines(t, "Pensantus's player (E6-15)", lines(a.log(t, a.ana, e)), []string{
		"R2 Pensantus anda 10 ft",
		"R2 Capitão Goblin -> Toren (Arco curto): acertou, 5 perfurante, aplicado",
		"R2 Brisa -> Capitão Goblin (Rapieira): acertou, 8 perfurante, aplicado",
		"R1 Toren -> Goblin 1 (Machado de batalha): acertou, 9 cortante, aplicado, derrotado",
		"R1 Goblin 2 -> Brisa (Arco curto): acertou, 5 perfurante, aplicado",
		"R1 Capitão Goblin -> Toren (Cimitarra): acertou, 5 cortante, aplicado",
		"R1 Brisa: Esconder",
		"R1 o combate começa",
	})
	attackAndApply(a.ana, "Pensantus", fireBolt, "Goblin 2", 13, 7) // 1d20 (13) + 6 = 19; 1d10 (7) de fogo
	end(a.ana)
	// 4. e 5. Goblin 1 e 2 derrotados: o turno passa direto. 6. Goblin 3: o mestre revela;
	// crítico de 1d20 (20): 2d6 (5, 4) + 2 = 11.
	if cur := a.get(t, a.master).GetCurrentCombatantId(); cur != a.id(t, "Goblin 3") {
		t.Fatalf("on turn = %s, want Goblin 3 (the defeated goblins are skipped)", cur)
	}
	if _, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Goblin 3"), IdempotencyKey: newKey(), Hidden: false,
	})); err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}
	a.h.roller.queue(20, 5, 4)
	crit := a.mustAttack(t, a.master, e, "Goblin 3", sword, "Brisa", inAppRoll)
	if crit.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_CRITICAL_HIT || crit.GetPendingDamage().GetDiceCount() != 2 {
		t.Fatalf("Goblin 3's attack = %v, want a critical hit with 2 dice", crit)
	}
	dmg := a.mustDamage(t, a.master, e, crit.GetPendingDamage().GetId(), inAppDamage).GetPendingDamage()
	if dmg.GetAmount() != 11 {
		t.Fatalf("critical damage = %v, want 2d6 (5, 4) + 2 = 11", dmg)
	}
	if _, err := a.settle(t, a.master, e, dmg.GetId(), true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	if got := a.vitals(t, a.bri).GetHitPointsCurrent(); got != brisa-16 {
		t.Errorf("Brisa = %d, want %d (5 + 11 damage)", got, brisa-16)
	}
	end(a.master)

	// 7. Toren anda 4 quadrados (6 m) até o Capitão e ataca com o dado físico:
	// 16 + 5 = 21; 1d8 (6) + 3 = 9.
	a.setPhysical(t, a.caio)
	e = a.get(t, a.caio)
	if _, err := a.caio.combat.MoveCombatant(t.Context(), connect.NewRequest(&playv1.MoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), IdempotencyKey: newKey(), Col: 9, Row: 5,
	})); err != nil {
		t.Fatalf("MoveCombatant() error = %v", err)
	}
	physical := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Capitão Goblin", d20(16))
	if d := physical.GetRoll().GetD20(); d.GetTotal() != 21 || !d.GetPhysical() {
		t.Fatalf("Toren's roll = %v, want 16 + 5 = 21, physical", d)
	}
	a.mustDamage(t, a.caio, e, physical.GetPendingDamage().GetId(), typedDamage(6))
	if cur, _, _ := a.hp(t, "Capitão Goblin"); cur != 10 {
		t.Errorf("the Capitão has %d PV, want 10 of 27 (8 and 9 of damage)", cur)
	}

	// The master's log (latest first), with the lines only he sees.
	wantLines(t, "the master", lines(a.log(t, a.master, e)), []string{
		"R2 Toren -> Capitão Goblin (Machado de batalha): acertou, 9 cortante, aplicado",
		"R2 Toren anda 20 ft",
		"R2 Goblin 3 -> Brisa (Cimitarra): crítico, 11 cortante, aplicado",
		"R2 o mestre mostra Goblin 3 (escondido: false) [só o mestre vê]",
		"R2 Pensantus -> Goblin 2 (Raio de Fogo): acertou, 7 fogo, aplicado, derrotado",
		"R2 Pensantus anda 10 ft",
		"R2 Capitão Goblin -> Toren (Arco curto): acertou, 5 perfurante, aplicado",
		"R2 Brisa -> Capitão Goblin (Rapieira): acertou, 8 perfurante, aplicado",
		"R1 Toren -> Goblin 1 (Machado de batalha): acertou, 9 cortante, aplicado, derrotado",
		"R1 Goblin 3: Esconder [só o mestre vê]",
		"R1 Goblin 2 -> Brisa (Arco curto): acertou, 5 perfurante, aplicado",
		"R1 Goblin 1 encerrou a parte [só o mestre vê]", // the goblins tie at 12: a group of NPCs alone, the master's
		"R1 Capitão Goblin -> Toren (Cimitarra): acertou, 5 cortante, aplicado",
		"R1 Brisa: Esconder",
		"R1 o combate começa",
	})
	// A player's: the same, without what only the master sees.
	wantLines(t, "Toren's player", lines(a.log(t, a.caio, e)), []string{
		"R2 Toren -> Capitão Goblin (Machado de batalha): acertou, 9 cortante, aplicado",
		"R2 Toren anda 20 ft",
		"R2 Goblin 3 -> Brisa (Cimitarra): crítico, 11 cortante, aplicado",
		"R2 Pensantus -> Goblin 2 (Raio de Fogo): acertou, 7 fogo, aplicado, derrotado",
		"R2 Pensantus anda 10 ft",
		"R2 Capitão Goblin -> Toren (Arco curto): acertou, 5 perfurante, aplicado",
		"R2 Brisa -> Capitão Goblin (Rapieira): acertou, 8 perfurante, aplicado",
		"R1 Toren -> Goblin 1 (Machado de batalha): acertou, 9 cortante, aplicado, derrotado",
		"R1 Goblin 2 -> Brisa (Arco curto): acertou, 5 perfurante, aplicado",
		"R1 Capitão Goblin -> Toren (Cimitarra): acertou, 5 cortante, aplicado",
		"R1 Brisa: Esconder",
		"R1 o combate começa",
	})
}

// TestMR012_CombatActionsAuthorizationMatrix: who may call each method of the
// actions of a turn. Signed out: unauthenticated. Not a member, or a pending
// one: not_found. A player on the master's methods: permission_denied; on
// another player's combatant: permission_denied; on their own, never turned
// away by authorization. The master is never turned away.
func TestMR012_CombatActionsAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t) // Toren on turn, next to the goblin
	outsider := a.h.newUser("Intruso")
	pending := a.h.newUser("Pendente")
	a.h.joinPending(a.master, a.campaignID, pending)
	pending.createCharacter(t, a.campaignID, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Esperando")
	anonymous := a.h.anonymous()

	// Toren's attack hits, so there is a pending damage of his to roll.
	a.h.roller.queue(15)
	torenHit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	pid := torenHit.GetPendingDamage().GetId()
	campaign, enc := a.campaignID, e.GetId()
	toren, goblin := a.id(t, "Toren"), a.id(t, "Goblin")

	calls := map[string]func(u *user, ctx context.Context) error{
		"GetTurnOptions": func(u *user, ctx context.Context) error {
			_, err := u.combat.GetTurnOptions(ctx, connect.NewRequest(&playv1.GetTurnOptionsRequest{CampaignId: campaign, EncounterId: enc, CombatantId: toren}))
			return err
		},
		"RollAttack": func(u *user, ctx context.Context) error {
			_, err := u.combat.RollAttack(ctx, connect.NewRequest(&playv1.RollAttackRequest{
				CampaignId: campaign, EncounterId: enc, AttackerId: toren, AttackKey: battleaxe, TargetId: goblin, IdempotencyKey: newKey(), Roll: &playv1.RollAttackRequest_RollInApp{RollInApp: true},
			}))
			return err
		},
		"RollDamage": func(u *user, ctx context.Context) error {
			_, err := u.combat.RollDamage(ctx, connect.NewRequest(&playv1.RollDamageRequest{
				CampaignId: campaign, EncounterId: enc, PendingDamageId: pid, IdempotencyKey: newKey(), Roll: &playv1.RollDamageRequest_RollInApp{RollInApp: true},
			}))
			return err
		},
		"ApplyPendingDamage": func(u *user, ctx context.Context) error {
			_, err := u.combat.ApplyPendingDamage(ctx, connect.NewRequest(&playv1.ApplyPendingDamageRequest{CampaignId: campaign, EncounterId: enc, PendingDamageId: pid, IdempotencyKey: newKey()}))
			return err
		},
		"DiscardPendingDamage": func(u *user, ctx context.Context) error {
			_, err := u.combat.DiscardPendingDamage(ctx, connect.NewRequest(&playv1.DiscardPendingDamageRequest{CampaignId: campaign, EncounterId: enc, PendingDamageId: pid, IdempotencyKey: newKey()}))
			return err
		},
		"TakeAction": func(u *user, ctx context.Context) error {
			_, err := u.combat.TakeAction(ctx, connect.NewRequest(&playv1.TakeActionRequest{CampaignId: campaign, EncounterId: enc, CombatantId: toren, ActionKey: "standard:dodge", IdempotencyKey: newKey()}))
			return err
		},
		"AdjustCombatantHitPoints": func(u *user, ctx context.Context) error {
			_, err := u.combat.AdjustCombatantHitPoints(ctx, connect.NewRequest(&playv1.AdjustCombatantHitPointsRequest{
				CampaignId: campaign, EncounterId: enc, CombatantId: goblin, IdempotencyKey: newKey(), Change: &playv1.AdjustCombatantHitPointsRequest_Heal{Heal: 1},
			}))
			return err
		},
		"UndoLastAction": func(u *user, ctx context.Context) error {
			_, err := u.combat.UndoLastAction(ctx, connect.NewRequest(&playv1.UndoLastActionRequest{CampaignId: campaign, EncounterId: enc, ExpectedEventId: newKey(), IdempotencyKey: newKey()}))
			return err
		},
		"ListCombatLog": func(u *user, ctx context.Context) error {
			_, err := u.combat.ListCombatLog(ctx, connect.NewRequest(&playv1.ListCombatLogRequest{CampaignId: campaign, EncounterId: enc}))
			return err
		},
	}
	methods := playv1.File_meurpg_play_v1_combat_proto.Services().ByName("CombatService").Methods()
	for _, name := range actionRPCs {
		if calls[name] == nil || methods.ByName(protoreflect.Name(name)) == nil {
			t.Errorf("the matrix lacks %s, or the service has no such method", name)
		}
	}
	if len(calls) != len(actionRPCs) {
		t.Errorf("the matrix has %d methods, want %d", len(calls), len(actionRPCs))
	}

	masterOnly := map[string]bool{"ApplyPendingDamage": true, "DiscardPendingDamage": true, "AdjustCombatantHitPoints": true, "UndoLastAction": true}
	anyMember := map[string]bool{"ListCombatLog": true}
	for name, call := range calls {
		if err := call(anonymous, t.Context()); connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("%s signed out: %v, want unauthenticated", name, err)
		}
		for who, u := range map[string]*user{"outsider": outsider, "pending": pending} {
			if err := call(u, t.Context()); connect.CodeOf(err) != connect.CodeNotFound {
				t.Errorf("%s as %s: %v, want not_found", name, who, err)
			}
		}
		// Pensantus's player, on Toren's things (or the master's).
		err := call(a.ana, t.Context())
		switch {
		case anyMember[name]:
			if err != nil {
				t.Errorf("%s as a player: %v, want ok", name, err)
			}
		default:
			if connect.CodeOf(err) != connect.CodePermissionDenied {
				t.Errorf("%s as another player: %v, want permission_denied", name, err)
			}
		}
	}
	// The owner is never told "not yours" nor "not found": only the rules may
	// refuse (the action may be used, the damage rolled...).
	for name, call := range calls {
		if masterOnly[name] {
			continue
		}
		if err := call(a.caio, t.Context()); connect.CodeOf(err) == connect.CodePermissionDenied || connect.CodeOf(err) == connect.CodeNotFound || connect.CodeOf(err) == connect.CodeUnauthenticated {
			t.Errorf("%s as the character's player: %v, want it past authorization", name, err)
		}
	}
	for name, call := range calls {
		if err := call(a.master, t.Context()); connect.CodeOf(err) == connect.CodePermissionDenied || connect.CodeOf(err) == connect.CodeUnauthenticated || connect.CodeOf(err) == connect.CodeNotFound {
			t.Errorf("%s as the master: %v, want it past authorization", name, err)
		}
	}
}

// events counts the session's events, to see that a retry writes none.
func (a *armed) events(t *testing.T) int {
	t.Helper()
	var n int
	if err := a.h.pool.QueryRow(t.Context(), `SELECT count(*) FROM session_events`).Scan(&n); err != nil {
		t.Fatalf("count the events: %v", err)
	}
	return n
}

// TestCombatActionsAreIdempotent: every write sent twice with the same key
// changes nothing the second time, writes no second event and answers as the
// first one did (a roll is never rolled again until it is good, RN-18).
func TestCombatActionsAreIdempotent(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	ctx := t.Context()
	toren := a.id(t, "Toren")

	// once runs call twice with one key, and checks the second left no event.
	once := func(name string, call func(key string) error) {
		t.Helper()
		key, before := newKey(), a.events(t)
		if err := call(key); err != nil {
			t.Fatalf("%s error = %v", name, err)
		}
		if got := a.events(t); got != before+1 {
			t.Fatalf("%s wrote %d events, want 1", name, got-before)
		}
		if err := call(key); err != nil {
			t.Fatalf("%s again error = %v", name, err)
		}
		if got := a.events(t); got != before+1 {
			t.Fatalf("the retry of %s wrote an event", name)
		}
	}

	// RollAttack: the same roll and the same pending damage, one action spent.
	a.h.roller.queue(15) // the retry would roll the next face (the damage's) if it rolled again
	var first, second *playv1.RollAttackResponse
	attackKey := newKey()
	call := func() (*playv1.RollAttackResponse, error) {
		res, err := a.caio.combat.RollAttack(ctx, connect.NewRequest(&playv1.RollAttackRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), AttackerId: toren, AttackKey: battleaxe, TargetId: a.id(t, "Goblin"),
			IdempotencyKey: attackKey, Roll: &playv1.RollAttackRequest_RollInApp{RollInApp: true},
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	var err error
	events := a.events(t)
	if first, err = call(); err != nil {
		t.Fatalf("RollAttack() error = %v", err)
	}
	if second, err = call(); err != nil {
		t.Fatalf("RollAttack() again error = %v", err)
	}
	if !proto.Equal(first.GetRoll(), second.GetRoll()) || first.GetPendingDamage().GetId() != second.GetPendingDamage().GetId() || first.GetRoll().GetD20().GetFaces()[0] != 15 {
		t.Errorf("the retry of RollAttack = %v, want the first answer %v", second, first)
	}
	if got := a.events(t); got != events+1 {
		t.Errorf("RollAttack wrote %d events, want 1", got-events)
	}
	// The key belongs to that change: another kind of change cannot take it.
	_, err = a.caio.combat.RollDamage(ctx, connect.NewRequest(&playv1.RollDamageRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: first.GetPendingDamage().GetId(), IdempotencyKey: attackKey,
		Roll: &playv1.RollDamageRequest_RollInApp{RollInApp: true},
	}))
	wantCode(t, "RollDamage with the attack's key", err, connect.CodeInvalidArgument)

	// RollDamage, with a goblin of 7 hit points: 1d8 (2) + 3 = 5, once.
	a.h.roller.queue(2)
	damageKey := newKey()
	var dmg1, dmg2 *playv1.RollDamageResponse
	rollDamage := func() (*playv1.RollDamageResponse, error) {
		res, err := a.caio.combat.RollDamage(ctx, connect.NewRequest(&playv1.RollDamageRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: first.GetPendingDamage().GetId(), IdempotencyKey: damageKey,
			Roll: &playv1.RollDamageRequest_RollInApp{RollInApp: true},
		}))
		if err != nil {
			return nil, err
		}
		return res.Msg, nil
	}
	if dmg1, err = rollDamage(); err != nil {
		t.Fatalf("RollDamage() error = %v", err)
	}
	if dmg2, err = rollDamage(); err != nil {
		t.Fatalf("RollDamage() again error = %v", err)
	}
	if !proto.Equal(dmg1.GetPendingDamage(), dmg2.GetPendingDamage()) || dmg2.GetPendingDamage().GetAmount() != 5 {
		t.Errorf("the retry of RollDamage = %v, want the first answer %v", dmg2.GetPendingDamage(), dmg1.GetPendingDamage())
	}
	if cur, _, _ := a.hp(t, "Goblin"); cur != 2 {
		t.Errorf("goblin = %d PV after the damage and its retry, want 2: applied once", cur)
	}

	// TakeAction, AdjustCombatantHitPoints: once each.
	a.mustEndTurn(t, a.caio, e) // Pensantus
	once("TakeAction", func(key string) error {
		_, err := a.ana.combat.TakeAction(ctx, connect.NewRequest(&playv1.TakeActionRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Pensantus"), ActionKey: "standard:dash", IdempotencyKey: key,
		}))
		return err
	})
	if mv := a.mustOptions(t, a.ana, e, "Pensantus").GetOptions().GetEconomy().GetMovement(); mv.GetSpeedFt() != 50 {
		t.Errorf("movement after Disparada and its retry = %v, want 50 (doubled once)", mv)
	}
	once("AdjustCombatantHitPoints", func(key string) error {
		_, err := a.master.combat.AdjustCombatantHitPoints(ctx, connect.NewRequest(&playv1.AdjustCombatantHitPointsRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Goblin"), IdempotencyKey: key, Change: &playv1.AdjustCombatantHitPointsRequest_Heal{Heal: 1},
		}))
		return err
	})
	if cur, _, _ := a.hp(t, "Goblin"); cur != 3 {
		t.Errorf("goblin = %d PV after healing 1 twice with one key, want 3", cur)
	}

	// The goblin hits Toren: Apply and Discard, once each.
	a.mustEndTurn(t, a.ana, e) // Brisa
	a.mustEndTurn(t, a.bia, e) // the goblin
	torenHP := a.vitals(t, a.toren).GetHitPointsCurrent()
	a.h.roller.queue(15, 3)
	hit := a.mustAttack(t, a.master, e, "Goblin", sword, "Toren", inAppRoll)
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage)
	once("ApplyPendingDamage", func(key string) error {
		_, err := a.master.combat.ApplyPendingDamage(ctx, connect.NewRequest(&playv1.ApplyPendingDamageRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: hit.GetPendingDamage().GetId(), IdempotencyKey: key,
		}))
		return err
	})
	if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != torenHP-5 {
		t.Errorf("Toren = %d after the apply and its retry, want %d: applied once", got, torenHP-5)
	}
	a.h.roller.queue(15, 3)
	hit = a.mustAttack(t, a.master, e, "Goblin", sword, "Toren", inAppRoll)
	a.mustDamage(t, a.master, e, hit.GetPendingDamage().GetId(), inAppDamage)
	once("DiscardPendingDamage", func(key string) error {
		_, err := a.master.combat.DiscardPendingDamage(ctx, connect.NewRequest(&playv1.DiscardPendingDamageRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), PendingDamageId: hit.GetPendingDamage().GetId(), IdempotencyKey: key,
		}))
		return err
	})

	// UndoLastAction: one step back, whatever the retries.
	last := a.log(t, a.master, e).GetUndoableEventId()
	once("UndoLastAction", func(key string) error {
		_, err := a.master.combat.UndoLastAction(ctx, connect.NewRequest(&playv1.UndoLastActionRequest{
			CampaignId: a.campaignID, EncounterId: e.GetId(), ExpectedEventId: last, IdempotencyKey: key,
		}))
		return err
	})
	if got := a.log(t, a.master, e).GetUndoableEventId(); got == last || got == "" {
		t.Errorf("the last action after one undo and its retry = %q, want the one before %q", got, last)
	}
}

// TestCombatLogChangedPerAudience: the stream's `combat_log_changed` reaches the
// master for every change, and a player only for a line they may see: a
// hidden goblin's attack never pings their stream (RN-10).
func TestCombatLogChangedPerAudience(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	// The hidden goblin plays first, then Toren.
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.goblin.GetId()}},
		npcRolls: []int{20},
		players:  map[string]int32{"Toren": 10, "Pensantus": 5, "Brisa": 1},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	masterStream, playerStream := a.master.watch(t, a.campaignID), a.ana.watch(t, a.campaignID)
	masterStream.ready(t)
	playerStream.ready(t)
	goblin := a.id(t, "Goblin")

	a.h.roller.queue(1)                                                       // a natural 1 misses: nothing is left open to hold the turn
	miss := a.mustAttack(t, a.master, e, "Goblin", sword, "Toren", inAppRoll) // 1. hidden: the master's hint only
	if miss.GetRoll().GetOutcome() != playv1.AttackOutcome_ATTACK_OUTCOME_MISS || miss.GetPendingDamage() != nil {
		t.Fatalf("attack = %v, want a miss", miss)
	}
	a.mustEndTurn(t, a.master, e)
	if _, err := a.caio.combat.TakeAction(t.Context(), connect.NewRequest(&playv1.TakeActionRequest{ // 2. in plain sight: both
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Toren"), ActionKey: "standard:dodge", IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("TakeAction() error = %v", err)
	}
	a.mustEndTurn(t, a.caio, e) // the sentinel: Pensantus's turn

	// Count what each stream got until the second turn_changed (the first is
	// the master's EndTurn).
	count := func(w *watcher) (logHints, turns int) {
		for turns < 2 {
			ev := w.nextChange(t)
			switch {
			case ev.GetCombatLogChanged() != nil:
				logHints++
				if ev.GetCombatLogChanged().GetEncounterId() != e.GetId() {
					t.Errorf("combat_log_changed = %v, want the combat's ID", ev)
				}
			case ev.GetTurnChanged() != nil:
				turns++
			}
			if strings.Contains(protojson.Format(ev), goblin) && w == playerStream {
				t.Fatalf("the player's stream has the hidden goblin's ID: %v", ev)
			}
		}
		return logHints, turns
	}
	if hints, _ := count(masterStream); hints != 2 {
		t.Errorf("the master got %d combat_log_changed, want 2 (the hidden attack and Toren's action)", hints)
	}
	if hints, _ := count(playerStream); hints != 1 {
		t.Errorf("the player got %d combat_log_changed, want 1 (Toren's action; not the hidden goblin's attack)", hints)
	}
}

// TestEndTurnDiscardShowsTheDroppedAttackInTheLog: passing the turn over an
// open damage writes a damage_discarded event for it, so the log shows the attack
// as dropped, not as waiting for ever, and the turn passed closes the undo.
func TestEndTurnDiscardShowsTheDroppedAttackInTheLog(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: a.capitao.GetId(), Hidden: new(false)}},
		npcRolls: []int{19},
		players:  map[string]int32{"Toren": 12, "Pensantus": 8, "Brisa": 1},
		at:       map[string][2]int32{"Capitão Goblin": {6, 3}, "Toren": {5, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}},
	})
	a.h.roller.queue(15)
	a.mustAttack(t, a.master, e, "Capitão Goblin", sword, "Toren", inAppRoll) // hit, damage never rolled
	if _, err := a.endTurn(t, a.master, e, true); err != nil {
		t.Fatalf("EndTurn(discard) error = %v", err)
	}
	for who, u := range map[string]*user{"master": a.master, "player": a.caio} {
		l := lines(a.log(t, u, e))
		if want := "R1 Capitão Goblin -> Toren (Cimitarra): acertou, 0 , descartado"; !slices.Contains(l, want) {
			t.Errorf("%s's log = %v, want %q", who, l, want)
		}
	}
	if id := a.log(t, a.master, e).GetUndoableEventId(); id != "" {
		t.Errorf("undoable event after the turn passed = %q, want none", id)
	}
}

// TestRN20_PendingDamageOfAHiddenTargetIsTheMasters: a player who hit an NPC
// that the master then hides learns nothing more of it: not its pending damage,
// not the damage roll, not its defeat. The master still resolves it.
func TestRN20_PendingDamageOfAHiddenTargetIsTheMasters(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	a.h.roller.queue(15)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	id, goblin := hit.GetPendingDamage().GetId(), a.id(t, "Goblin")
	if _, err := a.master.combat.SetCombatantHidden(t.Context(), connect.NewRequest(&playv1.SetCombatantHiddenRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: goblin, IdempotencyKey: newKey(), Hidden: true,
	})); err != nil {
		t.Fatalf("SetCombatantHidden() error = %v", err)
	}
	o := a.mustOptions(t, a.caio, a.get(t, a.caio), "Toren")
	if len(o.GetPendingDamages()) != 0 || strings.Contains(asJSON(t, o), goblin) {
		t.Errorf("a player's options = %s, want no word of the hidden goblin", asJSON(t, o))
	}
	_, err := a.damage(t, a.caio, e, id, inAppDamage)
	wantCode(t, "RollDamage on a hidden target", err, connect.CodeNotFound)
	if cur, _, _ := a.hp(t, "Goblin"); cur != 7 {
		t.Errorf("the goblin has %d PV, want 7: nothing was applied", cur)
	}
	a.h.roller.queue(8)
	if res := a.mustDamage(t, a.master, e, id, inAppDamage); res.GetPendingDamage().GetStatus() != playv1.PendingDamageStatus_PENDING_DAMAGE_STATUS_APPLIED {
		t.Errorf("the master's RollDamage = %v, want applied", res)
	}
}

// TestVitalsAreReadInsideTheTransaction: a change that computes from the vitals
// reads what its own transaction wrote, not a stale copy from the pool.
func TestVitalsAreReadInsideTheTransaction(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	ctx := t.Context()
	// The transaction has a connection of its own: the service's pool has one, and
	// the vitals read below goes through the service.
	tx, err := dbtest.SideConnection(t, a.h.pool).Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // the test never commits
	hp := a.vitals(t, a.toren).GetHitPointsCurrent() - 3
	if _, _, err := a.h.svc.vitals.AdjustVitals(ctx, tx, a.campaignID, a.toren.GetId(), &playv1.AdjustCharacterVitalsRequest{HitPointsCurrent: &hp}); err != nil {
		t.Fatalf("AdjustVitals() error = %v", err)
	}
	in, err := a.h.svc.vitals.GetVitalsTx(ctx, tx, a.campaignID, a.toren.GetId())
	if err != nil || in.GetHitPointsCurrent() != hp {
		t.Errorf("GetVitalsTx() = %v, %v; want %d, the transaction's own write", in, err, hp)
	}
}

// TestCombatLogKeepsTheNewestEvents: a combat longer than the log's limit loses
// its oldest lines, never the newest, nor the one an undo would take back.
//
// Not parallel: it lowers the package's logEventLimit, which every combat log
// reads. Go holds the parallel tests until the sequential ones end, so none of
// them reads the log while the limit is 1 (it was a data race, and other
// tests' logs came back with a single line now and then).
func TestCombatLogKeepsTheNewestEvents(t *testing.T) {
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	if _, err := a.action(t, a.caio, e, "Toren", "standard:dodge"); err != nil {
		t.Fatalf("TakeAction() error = %v", err)
	}
	old := logEventLimit
	logEventLimit = 1
	t.Cleanup(func() { logEventLimit = old })
	l := lines(a.log(t, a.master, e))
	if len(l) != 1 || l[0] != "R1 Toren: Esquivar" || a.log(t, a.master, e).GetUndoableEventId() == "" {
		t.Errorf("log with a limit of 1 = %v, want only the newest line, Esquivar, still undoable", l)
	}
}

// TestCombatLogHidesEntriesOfCombatantsThatLeft: a combatant removed from the
// combat leaves its lines anonymous, and a player does not get those.
func TestCombatLogHidesEntriesOfCombatantsThatLeft(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	a.h.roller.queue(15, 2)
	hit := a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", inAppRoll)
	a.mustDamage(t, a.caio, e, hit.GetPendingDamage().GetId(), inAppDamage)
	if _, err := a.master.combat.RemoveCombatant(t.Context(), connect.NewRequest(&playv1.RemoveCombatantRequest{
		CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, "Goblin"), IdempotencyKey: newKey(),
	})); err != nil {
		t.Fatalf("RemoveCombatant() error = %v", err)
	}
	for _, r := range a.log(t, a.caio, e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK {
				t.Errorf("a player got an anonymous entry: %v", en)
			}
		}
	}
	found := false
	for _, r := range a.log(t, a.master, e).GetRounds() {
		for _, en := range r.GetEntries() {
			if en.GetKind() == playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK && en.GetHidden() {
				found = true
			}
		}
	}
	if !found {
		t.Error("the master's log lacks the entry, marked as his alone")
	}
}

// TestUndoOfAppliedDamageSurvivesALowerMaximum: the maximum hit points dropped
// since the damage was applied (a level lost), and the undo puts back what the
// sheet allows instead of being refused for ever.
func TestUndoOfAppliedDamageSurvivesALowerMaximum(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.threeAndAGoblin(t)
	a.mustEndTurn(t, a.caio, e)
	a.mustEndTurn(t, a.ana, e)
	a.mustEndTurn(t, a.bia, e) // the goblin
	a.h.roller.queue(15, 3)
	hit := a.mustAttack(t, a.master, e, "Goblin", sword, "Toren", inAppRoll)
	id := hit.GetPendingDamage().GetId()
	a.mustDamage(t, a.master, e, id, inAppDamage)
	if _, err := a.settle(t, a.master, e, id, true); err != nil {
		t.Fatalf("ApplyPendingDamage() error = %v", err)
	}
	if _, err := a.h.pool.Exec(t.Context(),
		`UPDATE characters SET sheet = jsonb_set(sheet, '{full,classes,0,level}', '1') WHERE id = $1`, a.toren.GetId()); err != nil {
		t.Fatalf("lower the level: %v", err)
	}
	newMax := a.vitals(t, a.toren).GetHitPointsMax()
	if err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId()); err != nil {
		t.Fatalf("UndoLastAction() error = %v, want it cut to the new maximum", err)
	}
	if got := a.vitals(t, a.toren).GetHitPointsCurrent(); got != newMax {
		t.Errorf("Toren = %d, want the new maximum %d", got, newMax)
	}
}

// TestRN18_PlayersChooseOnEachRoll: with "cada jogador escolhe" the saved
// preference only says what the screen offers: a player whose preference is
// real dice may roll in the app, and one whose preference is the app may type a
// face. A forced mode is refused the other way (see TestRN18_PhysicalRollsAreTypedSums).
func TestRN18_PlayersChooseOnEachRoll(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e := a.start(t, plan{npcs: []*playv1.Participant{{CharacterId: a.goblin.GetId()}}, npcRolls: []int{3}, setup: true})
	a.setPhysical(t, a.caio) // Toren prefers real dice; Pensantus has the default, the app
	submit := func(u *user, label string, roll func(*playv1.SubmitInitiativeRequest)) error {
		req := &playv1.SubmitInitiativeRequest{CampaignId: a.campaignID, EncounterId: e.GetId(), CombatantId: a.id(t, label), IdempotencyKey: newKey()}
		roll(req)
		_, err := u.combat.SubmitInitiative(t.Context(), connect.NewRequest(req))
		return err
	}
	if err := submit(a.caio, "Toren", inApp); err != nil {
		t.Errorf("the app's roll for a player who prefers real dice = %v, want ok", err)
	}
	if err := submit(a.ana, "Pensantus", typed(12)); err != nil {
		t.Errorf("a typed face for a player who prefers the app = %v, want ok", err)
	}
}

// A reach weapon (glaive) attacks a target two squares away.
func TestAReachWeaponHitsAtTenFeet(t *testing.T) {
	a := newArmedWith(t, func(a *armed) {
		a.toren = a.caio.hero(t, a.campaignID, "Toren", "class:fighter", "race:human", 2,
			&rulesv1.AbilityScores{Strength: 15, Dexterity: 13, Constitution: 14, Intelligence: 10, Wisdom: 10, Charisma: 8},
			[]string{"equipment:glaive"}, nil)
	})
	goblin := a.master.sizedNPC(t, a.campaignID, "Goblin", 7, 12, rulesv1.CreatureSize_CREATURE_SIZE_SMALL)
	a.mapID = a.h.newMapOf(a.campaignID, 24, 1200, 800)
	if _, err := a.master.play.SetCurrentMap(t.Context(), connect.NewRequest(&playv1.SetCurrentMapRequest{CampaignId: a.campaignID, MapId: a.mapID})); err != nil {
		t.Fatalf("SetCurrentMap() error = %v", err)
	}
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: goblin.GetId()}},
		npcRolls: []int{2},
		players:  map[string]int32{"Toren": 18},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {5, 3}},
	})
	if _, err := a.attack(t, a.caio, e, "Toren", "equipment:glaive", "Goblin", d20(12)); err != nil {
		t.Errorf("glaive (Reach, 10 ft) attack on a goblin two squares away = %v, want it allowed", err)
	}
}

// An undo whose resource the sheet no longer has (the level was lowered meanwhile) is handled,
// never an internal error that blocks the undo chain.
func TestUndoAfterTheSheetLostTheResourceIsNotAnInternalError(t *testing.T) {
	t.Parallel()
	a := newCasters(t)
	e := a.castersFight(t, 1)
	a.mustEndTurn(t, a.ana, e) // Toren's turn
	if _, err := a.feature(t, a.caio, e, "Toren", actionSurgeKey, noTakeRoll); err != nil {
		t.Fatalf("TakeAction(action surge) error = %v", err)
	}
	if _, err := a.h.pool.Exec(t.Context(),
		`UPDATE characters SET sheet = jsonb_set(sheet, '{full,classes,0,level}', '1') WHERE id = $1`, a.toren.GetId()); err != nil {
		t.Fatalf("lower the level: %v", err)
	}
	err := a.undo(t, a.master, e, a.log(t, a.master, e).GetUndoableEventId())
	if err != nil && connect.CodeOf(err) == connect.CodeInternal {
		t.Fatalf("UndoLastAction() = %v, want a handled outcome, not an internal error", err)
	}
}
