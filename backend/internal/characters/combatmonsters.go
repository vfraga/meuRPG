package characters

import (
	"context"
	"crypto/sha1" //nolint:gosec // a name-based UUID (RFC 9562, version 5), not a security hash
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The monsters of "Pôr no combate" (MR-042, RN-29). The app keeps one NPC of the
// campaign for each creature the master has put in a combat, made here from the
// creature (the same sheet "Criar NPC" makes) and marked combat_only, so the
// master's list never shows it. Package play makes a combatant of each monster as
// a copy of that NPC, with its own hit points and name; everything that works for
// an NPC (RN-20, hidden NPCs, the XP by enemies) works for the monster.

// hitPointsRoll reads the SRD's "2d8+2", "33d20+330" or "5d10".
var hitPointsRoll = regexp.MustCompile(`^(\d+)d(\d+)(?:\s*([+-])\s*(\d+))?$`)

// monsterNamespace is the namespace of the UUIDs of the monsters' NPCs.
var monsterNamespace = uuid.MustParse("6f1d6a52-3c1e-4d0b-9a57-0c7b5e1f2a43")

// monsterNpcKey is the create_key of the NPC of a creature in a campaign: a
// version 5 UUID of both, so the same pair always has the same key, and the
// unique index (campaign_id, create_key) makes the NPC one, whoever creates it.
func monsterNpcKey(campaignID, monsterKey string) string {
	return nameUUID(monsterNamespace, campaignID+"|"+monsterKey)
}

// nameUUID is the version 5 UUID (RFC 9562) of name in namespace.
func nameUUID(namespace uuid.UUID, name string) string {
	h := sha1.New() //nolint:gosec // see the import
	h.Write(namespace[:])
	h.Write([]byte(name))
	sum := h.Sum(nil)
	var u uuid.UUID
	copy(u[:], sum[:16])
	u[6] = u[6]&0x0f | 0x50 // version 5
	u[8] = u[8]&0x3f | 0x80 // the RFC variant
	return u.String()
}

// MonsterHitPoints implements play.CombatRoster.
func (s *Service) MonsterHitPoints(ctx context.Context, tx pgx.Tx, campaignID, monsterKey string) (link.MonsterHitPoints, bool, error) {
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return link.MonsterHitPoints{}, false, s.dbError(ctx, "read rules content", err)
	}
	out, ok := monsterHitPoints(content, monsterKey)
	return out, ok, nil
}

// monsterHitPoints reads the creature's hit points from the content.
func monsterHitPoints(content *rules.Content, monsterKey string) (link.MonsterHitPoints, bool) {
	c, ok := content.CreatureByKey(monsterKey)
	if !ok {
		return link.MonsterHitPoints{}, false
	}
	out := link.MonsterHitPoints{Average: c.HitPoints}
	// A roll the SRD wrote in another shape keeps only the average.
	if m := hitPointsRoll.FindStringSubmatch(c.HitPointsRoll); m != nil {
		count, _ := strconv.Atoi(m[1])
		sides, _ := strconv.Atoi(m[2])
		bonus, _ := strconv.Atoi(m[4])
		if m[3] == "-" {
			bonus = -bonus
		}
		out.DiceCount, out.DiceSides, out.DiceBonus = count, sides, bonus
	}
	return out, true
}

// MonsterNpc implements play.CombatRoster: the NPC of the creature in the
// campaign, made the first time. The insert is ON CONFLICT DO NOTHING on the
// fixed key, so two calls at once make one, and the read after it, in the same
// transaction, finds the row whoever wrote it.
func (s *Service) MonsterNpc(ctx context.Context, tx pgx.Tx, campaignID, masterUserID, monsterKey string, at time.Time) (link.Character, bool, error) {
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return link.Character{}, false, s.dbError(ctx, "read rules content", err)
	}
	c, ok := content.CreatureByKey(monsterKey)
	if !ok {
		return link.Character{}, false, nil
	}
	q := s.queriesIn(tx)
	key := monsterNpcKey(campaignID, monsterKey)
	row, err := q.GetCharacterByCreateKey(ctx, charactersdb.GetCharacterByCreateKeyParams{CampaignID: campaignID, CreateKey: key})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return link.Character{}, false, wrap("read the NPC of a monster", err)
	}
	if err != nil {
		basic, err := npcSheetFromCreature(content, monsterKey)
		if err != nil {
			return link.Character{}, false, err
		}
		basic.CombatOnly = true
		sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Basic{Basic: basic}}
		params := charactersdb.InsertNpcFromCreatureParams{
			CampaignID: campaignID, Kind: kindMinion, MasterUserID: &masterUserID, Name: c.NamePT, CreateKey: key, Now: at,
		}
		if params.Sheet, err = storeJSON.Marshal(sheet); err != nil {
			return link.Character{}, false, fmt.Errorf("encode a sheet: %w", err)
		}
		if params.Story, err = storeJSON.Marshal(&charactersv1.CharacterStory{}); err != nil {
			return link.Character{}, false, fmt.Errorf("encode a story: %w", err)
		}
		if row, err = q.InsertNpcFromCreature(ctx, params); err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return link.Character{}, false, wrap("insert the NPC of a monster", err)
			}
			// Another transaction made it first: read it.
			if row, err = q.GetCharacterByCreateKey(ctx, charactersdb.GetCharacterByCreateKeyParams{CampaignID: campaignID, CreateKey: key}); err != nil {
				return link.Character{}, false, wrap("read the NPC of a monster", err)
			}
		}
	}
	out, err := combatCharacter(content, row.ID, row.Kind, row.Name, nil, row.Sheet, nil)
	if err != nil {
		return link.Character{}, false, err
	}
	return out, true, nil
}
