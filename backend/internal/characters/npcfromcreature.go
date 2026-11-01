package characters

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// "Criar NPC" from a creature of the bestiary (MR-042, RN-29): a named NPC with
// a basic sheet filled in from the SRD stat block. The NPC is a copy, so the
// bestiary never changes and the master edits the NPC like any other.

// CreateNpcFromCreature implements charactersv1connect.CharacterServiceHandler.
func (s *Service) CreateNpcFromCreature(
	ctx context.Context,
	req *connect.Request[charactersv1.CreateNpcFromCreatureRequest],
) (*connect.Response[charactersv1.CreateNpcFromCreatureResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	params, err := checkNpcFromCreature(req.Msg, m)
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID) // before the write, as the other creates
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	basic, err := npcSheetFromCreature(content, req.Msg.GetCreatureKey())
	if err != nil {
		return nil, invalidArgument(err)
	}
	params.Now = s.now()
	if what, err := encodeNpcFromCreature(&params, basic); err != nil {
		return nil, s.dbError(ctx, what, err)
	}

	var row charactersdb.Character
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		row, err = s.insertNpcFromCreature(ctx, s.queries.WithTx(tx), params, basic.GetMonsterKey())
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "create an NPC from a creature", err)
	}
	c, err := s.character(ctx, content, row, m)
	if err != nil {
		return nil, s.dbError(ctx, "read a new character", err)
	}
	return connect.NewResponse(&charactersv1.CreateNpcFromCreatureResponse{Character: c}), nil
}

// checkNpcFromCreature checks the request's kind, name and idempotency key, in
// that order, and starts the row of the NPC with them.
func checkNpcFromCreature(msg *charactersv1.CreateNpcFromCreatureRequest, m authz.Membership) (charactersdb.InsertNpcFromCreatureParams, error) {
	// A basic sheet is the MINION's and the STORY NPC's; ENEMY and BOSS take a
	// full sheet, which a creature cannot fill (it has no class or race).
	kind, ok := kindToDB[msg.GetKind()]
	if !ok || (kind != kindMinion && kind != kindStory) {
		return charactersdb.InsertNpcFromCreatureParams{}, invalidArgument(fieldErr("kind", "must be MINION or STORY"))
	}
	name, err := names.Clean(msg.GetName(), MaxNameLength)
	if err != nil {
		return charactersdb.InsertNpcFromCreatureParams{}, invalidArgument(&fieldError{field: "name", err: err})
	}
	createKey, ok := uid.Canonical(msg.GetIdempotencyKey())
	if !ok {
		return charactersdb.InsertNpcFromCreatureParams{}, invalidArgument(fieldErr("idempotency_key", "must be a UUID"))
	}
	return charactersdb.InsertNpcFromCreatureParams{CampaignID: m.CampaignID, Kind: kind, MasterUserID: &m.UserID, Name: name, CreateKey: createKey}, nil
}

// encodeNpcFromCreature writes the NPC's basic sheet and its empty story into the
// row. On a failure it also says what it was doing, for dbError.
func encodeNpcFromCreature(params *charactersdb.InsertNpcFromCreatureParams, basic *charactersv1.BasicSheet) (string, error) {
	sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Basic{Basic: basic}}
	var err error
	if params.Sheet, err = storeJSON.Marshal(sheet); err != nil {
		return "encode a sheet", err
	}
	if params.Story, err = storeJSON.Marshal(&charactersv1.CharacterStory{}); err != nil {
		return "encode a story", err
	}
	return "", nil
}

// insertNpcFromCreature writes the NPC inside the campaign's cap or, when its key
// was used, answers with the NPC an earlier call made, if the request is the same
// one (the same kind, name and creature).
func (s *Service) insertNpcFromCreature(ctx context.Context, q *charactersdb.Queries, params charactersdb.InsertNpcFromCreatureParams, monsterKey string) (charactersdb.Character, error) {
	row, err := q.InsertNpcFromCreature(ctx, params)
	if err == nil {
		return row, s.checkRoom(ctx, q, params.CampaignID, true)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return row, wrap("insert an NPC from a creature", err)
	}
	// The key was used: an earlier call made this NPC. Answer with it, if
	// the request is the same one.
	row, err = q.GetCharacterByCreateKey(ctx, charactersdb.GetCharacterByCreateKeyParams{CampaignID: params.CampaignID, CreateKey: params.CreateKey})
	if err != nil {
		return row, wrap("read the NPC of an idempotency key", err)
	}
	kept, err := loadSheet(row.ID, row.Sheet)
	if err != nil {
		return row, err
	}
	if row.Kind != params.Kind || row.Name != params.Name || kept.GetBasic().GetMonsterKey() != monsterKey {
		return row, invalidArgument(fieldErr("idempotency_key", "was already used for another NPC"))
	}
	return row, nil
}

// maxNpcAttacks is how many of a creature's attacks go to the sheet: the
// basic sheet holds three (maxBasicAttacks).
const maxNpcAttacks = maxBasicAttacks

// npcSheetFromCreature is the basic sheet of an NPC made from the creature with
// that key. The error is the field's, for invalid_argument.
func npcSheetFromCreature(content *rules.Content, key string) (*charactersv1.BasicSheet, error) {
	c, ok := content.CreatureByKey(key)
	if !ok {
		return nil, fieldErr("creature_key", "is not an SRD creature")
	}
	d, _ := content.MonsterDerived(key)
	// The walking speed; a creature that does not walk (a fish, a ghost) takes
	// its best other speed, so the NPC is not stuck.
	speed := c.SpeedWalkFt
	if speed == 0 {
		speed = max(c.SpeedFlyFt, c.SpeedSwimFt, c.SpeedClimbFt, c.SpeedBurrowFt)
	}
	basic := &charactersv1.BasicSheet{
		HitPointsMax: i32(c.HitPoints), ArmorClass: i32(c.ArmorClass), SpeedFt: i32(min(speed, maxSpeedFt)),
		Description:     "Baseado em " + c.NamePT + creatureOrigin(key),
		InitiativeBonus: i32(d.Initiative),
		ChallengeRating: c.ChallengeRating, XpValue: i32(c.XP),
		Size:       rulesv1.CreatureSize(rulesv1.CreatureSize_value["CREATURE_SIZE_"+strings.ToUpper(c.Size)]),
		MonsterKey: key,
	}
	scores := map[rules.Ability]int32{}
	for _, a := range c.Abilities {
		scores[a.Ability] = i32(a.Score)
	}
	basic.AbilityScores = &rulesv1.AbilityScores{
		Strength: scores[rules.STR], Dexterity: scores[rules.DEX], Constitution: scores[rules.CON],
		Intelligence: scores[rules.INT], Wisdom: scores[rules.WIS], Charisma: scores[rules.CHA],
	}
	// A basic attack holds one damage. The other parts of the action's damage
	// ("+2d6 fogo" on a dragon's bite) go into the description, so they are not
	// lost; the saving throw or rider of an action stays in the SRD stat block.
	var extras []string
	for _, a := range d.Attacks {
		if len(basic.Attacks) == maxNpcAttacks {
			break
		}
		at, ok := basicAttackOf(a)
		if !ok {
			continue
		}
		basic.Attacks = append(basic.Attacks, at)
		if more := extraDamage(c, a.Name); more != "" {
			extras = append(extras, at.Name+": "+more+".")
		}
	}
	if len(extras) > 0 {
		basic.Description += "\n" + strings.Join(extras, "\n")
	}
	// The multiattack count the turn already follows (through MonsterKey) and the
	// creature's spellcasting, written for the master as a note: the NPC's own spells in combat come from rules.Content.MonsterCaster.
	if d.AttacksPerAction > 1 {
		basic.Description += fmt.Sprintf("\nAtaques por ação: %d (ataque múltiplo).", d.AttacksPerAction)
	}
	if note := spellcastingNote(content, c); note != "" {
		basic.Description += "\n" + note
	}
	// The same checks as any basic sheet: a creature that breaks one is a bug in
	// the data, and the test over the 334 creatures says so.
	if err := cleanBasicSheet(basic); err != nil {
		return nil, err
	}
	return basic, nil
}

// extraDamage writes the damage parts of the creature's action called name that
// come after the first, as "+2d6 fogo", or "" when there is only one part.
func extraDamage(c rules.Creature, name string) string {
	for _, act := range c.Actions {
		if act.Name != name || !act.HasAttack || len(act.Damage) < 2 {
			continue
		}
		parts := make([]string, 0, len(act.Damage)-1)
		for _, d := range act.Damage[1:] {
			parts = append(parts, "+"+d.Dice+" "+strings.ToLower(d.TypeNamePT))
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

// basicAttackOf turns a creature's attack, weapon or spell, into a basic sheet's:
// the attack bonus, the reach or range, and the first damage part, rolled with a
// die the sheet has. An attack with no damage, or whose die the sheet has no
// face for (a d20, a d100, a flat number), is left out and stays in the stat
// block; so does a saving throw riding on a hit (a SaveActions entry of the
// creature, not an attack).
func basicAttackOf(a rules.Attack) (*charactersv1.BasicAttack, bool) {
	dmg, found := charactersv1.DamageType_value["DAMAGE_TYPE_"+strings.ToUpper(strings.TrimPrefix(a.DamageType, "damage-type:"))]
	d := a.DamageDice
	if (a.Kind != "weapon" && a.Kind != "spell") || !found || d.Count < 1 || d.Count > maxDamageDiceCount || !validDie(d.Sides) {
		return nil, false
	}
	// A melee attack at the normal reach is range 0, as the sheet writes it.
	reach := a.RangeFt
	if a.Melee && reach <= 5 {
		reach = 0
	}
	at := &charactersv1.BasicAttack{
		Name: a.NamePT, AttackBonus: i32(a.AttackBonus), DamageDiceCount: i32(d.Count), DamageDiceSides: i32(d.Sides),
		DamageBonus: i32(d.Bonus), DamageType: charactersv1.DamageType(dmg), RangeFt: i32(min(reach, maxRangeFt)),
	}
	if len([]rune(at.Name)) > maxAttackNameLength {
		at.Name = string([]rune(at.Name)[:maxAttackNameLength])
	}
	return at, true
}

func validDie(sides int) bool {
	return sides == 4 || sides == 6 || sides == 8 || sides == 10 || sides == 12
}

// keepCreatureLink is the sheet to save over current: when current is the sheet
// of an NPC made from a creature and the new one is a basic sheet, the new one
// keeps the creature's key and ability scores, which the client never sends.
func keepCreatureLink(current charactersdb.Character, sheet *charactersv1.CharacterSheet) *charactersv1.CharacterSheet {
	if sheet.GetBasic() == nil || current.Kind == kindPlayer {
		return sheet
	}
	saved, err := loadSheet(current.ID, current.Sheet)
	if err != nil || saved.GetBasic().GetMonsterKey() == "" {
		return sheet // nothing to keep (or an unreadable sheet, which the update replaces)
	}
	out := proto.CloneOf(sheet)
	out.GetBasic().MonsterKey = saved.GetBasic().GetMonsterKey()
	out.GetBasic().AbilityScores = saved.GetBasic().GetAbilityScores()
	out.GetBasic().CombatOnly = saved.GetBasic().GetCombatOnly()
	return out
}

// creatureOrigin is where the creature of an NPC comes from, for its description: the SRD, or the table.
func creatureOrigin(key string) string {
	if rules.IsTableKey(key) {
		return " (criatura da mesa)."
	}
	return " (SRD 5.1)."
}
