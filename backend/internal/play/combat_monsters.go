package play

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// "Pôr no combate" (MR-042, RN-29): monsters of an SRD creature join a combat.
// Each is an NPC combatant, linked to an NPC of its own that the characters
// module makes from the creature (hidden from the master's list), so the rest of
// the combat treats it as any NPC: the players see its state word and never its
// hit points or armor class (RN-20), a hidden one is not sent at all, its
// attacks go through RollAttack and its XP counts by the creature's CR (RN-09).

// maxMonsterName is the longest base name: a label is the name, a space and a
// number up to 10, and a label never passes maxLabelLength.
const maxMonsterName = 30

// monstersEvent is what the event of an add keeps, ids and numbers only (never a
// name: the name is kept as a hash, to check a retry).
type monstersEvent struct {
	CreatureKey string `json:"creature_key"`
	Count       int    `json:"count"`
	NameHash    string `json:"name_hash"`
	Rolled      bool   `json:"rolled"`
	Hidden      bool   `json:"hidden"`
	// Items are the new monsters, in the order of their labels.
	Items []monsterItem `json:"items"`
}

// monsterItem is one new monster: its hit points and, when they were rolled, the
// dice, their faces and the bonus.
type monsterItem struct {
	ID        string  `json:"id"`
	HitPoints int32   `json:"hit_points"`
	Dice      string  `json:"dice,omitempty"`
	Faces     []int32 `json:"faces,omitempty"`
	Modifier  int32   `json:"modifier,omitempty"`
}

// sameAdd says whether a retry asks for what the event kept.
func (e *monstersEvent) sameAdd(o monstersEvent) bool {
	return e.CreatureKey == o.CreatureKey && e.Count == o.Count && e.NameHash == o.NameHash && e.Rolled == o.Rolled && e.Hidden == o.Hidden
}

// AddMonsters implements playv1connect.CombatServiceHandler.
func (s *Service) AddMonsters(
	ctx context.Context,
	req *connect.Request[playv1.AddMonstersRequest],
) (*connect.Response[playv1.AddMonstersResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	encID, err := parseCombatID(req.Msg.GetEncounterId(), "encounter")
	if err != nil {
		return nil, err
	}
	count := int(req.Msg.GetCount())
	if count == 0 {
		count = 1
	}
	if count < 1 || count > maxNPCCopies {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("count must be 1 to %d", maxNPCCopies))
	}
	var rolled bool
	switch req.Msg.GetHitPoints() {
	case playv1.MonsterHitPoints_MONSTER_HIT_POINTS_UNSPECIFIED, playv1.MonsterHitPoints_MONSTER_HIT_POINTS_AVERAGE:
	case playv1.MonsterHitPoints_MONSTER_HIT_POINTS_ROLLED:
		rolled = true
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("hit_points is not a known mode"))
	}
	hidden := req.Msg.Hidden == nil || req.Msg.GetHidden() // a new monster starts hidden, as any NPC

	// What the creature is, read before the write (never through the pool inside it).
	creatureKey := req.Msg.GetCreatureKey()
	hp, ok, err := s.roster.MonsterHitPoints(ctx, nil, m.CampaignID, creatureKey)
	if err != nil {
		return nil, s.dbError(ctx, "read a creature", err)
	}
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("creature_key is not an SRD creature"))
	}
	name, err := s.monsterName(ctx, m.CampaignID, creatureKey, req.Msg.GetName(), nil)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(name))
	asked := monstersEvent{CreatureKey: creatureKey, Count: count, NameHash: hex.EncodeToString(sum[:8]), Rolled: rolled, Hidden: hidden}

	var done monstersEvent
	// No request hash: the add is compared by its own digest, which reads the defaults written out as the same request.
	res, err := s.write(ctx, combatWrite{m: m, key: key, kind: eventCombatantsAdded, encounterID: encID}, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		if len(cs)+count > maxCombatants {
			// Typed, so the app says it in words instead of guessing from an invalid argument.
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TOO_MANY_COMBATANTS, fmt.Sprintf("a combat has at most %d combatants", maxCombatants))
		}
		parts, batchItems, err := s.planMonsters(ctx, c, m, []monsterBatch{{creatureKey: creatureKey, name: name, count: count, hp: hp}}, rolled, hidden)
		if err != nil {
			return nil, err
		}
		items := batchItems[0]
		_, news, err := s.addParticipants(ctx, c, link.Grid{Columns: c.enc.GridColumns, Rows: c.enc.GridRows}, cs, parts)
		if err != nil {
			return nil, err
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		done = asked
		done.Items = items
		for i, n := range news {
			done.Items[i].ID = n.ID
		}
		return actionEvent{Round: c.enc.Round, Monsters: &done}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "add monsters", err)
	}
	if res.repeated {
		// A retry: the same parameters get the same answer; any other is refused, as is
		// a key another kind of change (AddCombatants) used.
		ev, err := readEvent(res.payload)
		if err != nil || ev.Monsters == nil || !ev.Monsters.sameAdd(asked) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
		}
		done = *ev.Monsters
	}
	ids := make([]string, len(done.Items))
	for i, it := range done.Items {
		ids[i] = it.ID
	}
	out, err := s.finish(ctx, m, res, s.changedFor(m.CampaignID, ids...))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.AddMonstersResponse{Encounter: out, CombatantIds: ids}), nil
}

// monsterName is the base name of a creature's monsters: the master's, or the creature's
// Portuguese name, cut to maxMonsterName characters, checked as a one-line name.
// nameOf is the campaign's naming function; nil reads it (a caller with several names reads it once).
func (s *Service) monsterName(ctx context.Context, campaignID, creatureKey, given string, nameOf func(string) string) (string, error) {
	name := strings.TrimSpace(given)
	if name == "" {
		if nameOf == nil {
			var err error
			if nameOf, err = s.roster.ContentNames(ctx, nil, campaignID); err != nil {
				return "", s.dbError(ctx, "read the content names", err)
			}
		}
		pt := []rune(nameOf(creatureKey))
		name = string(pt[:min(maxMonsterName, len(pt))])
	}
	name, err := names.Clean(name, maxMonsterName)
	if err != nil {
		return "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name: %w", err))
	}
	return name, nil
}

// monsterBatch is the monsters of one creature, checked and ready to join a combat.
type monsterBatch struct {
	creatureKey, name string
	count             int
	hp                link.MonsterHitPoints
}

// planMonsters makes, inside the change's transaction, the NPC of each creature (made the
// first time, reused after) and the hit points of each monster, and returns the participants
// to add and, per batch, each monster's hit points (and dice, when rolled). The hit dice of
// every monster are rolled here, before addParticipants rolls the d20 of their initiative. The
// same code serves AddMonsters and a start with monsters (StartEncounter).
func (s *Service) planMonsters(ctx context.Context, c *combatTx, m authz.Membership, batches []monsterBatch, rolled, hidden bool) ([]planned, [][]monsterItem, error) {
	parts := make([]planned, 0, len(batches))
	all := make([][]monsterItem, 0, len(batches))
	for _, b := range batches {
		npc, ok, err := s.roster.MonsterNpc(ctx, c.tx, m.CampaignID, m.UserID, b.creatureKey, c.now)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			return nil, nil, connect.NewError(connect.CodeInvalidArgument, errors.New("creature_key is not an SRD creature"))
		}
		items := make([]monsterItem, b.count)
		points := make([]int, b.count)
		for i := range items {
			items[i].HitPoints = int32(b.hp.Average) //nolint:gosec // a creature's average, well inside int32
			if rolled && b.hp.DiceCount > 0 {
				if items[i], err = s.rollMonsterHitPoints(b.hp); err != nil {
					return nil, nil, err
				}
			}
			points[i] = int(items[i].HitPoints)
		}
		parts = append(parts, planned{char: npc, count: b.count, hidden: hidden, name: b.name, hitPoints: points})
		all = append(all, items)
	}
	return parts, all, nil
}

// rollMonsterHitPoints rolls a creature's hit dice, in the app, and adds its
// bonus: at least 1.
func (s *Service) rollMonsterHitPoints(hp link.MonsterHitPoints) (monsterItem, error) {
	res, err := dice.Roll(s.roller, dice.Expr{Count: hp.DiceCount, Sides: hp.DiceSides})
	if err != nil {
		return monsterItem{}, fmt.Errorf("roll the hit points of a monster: %w", err)
	}
	faces := make([]int32, len(res.Faces))
	for i, f := range res.Faces {
		faces[i] = clamp32(f, 1, 1000)
	}
	expr := dice.Expr{Count: hp.DiceCount, Sides: hp.DiceSides, Modifier: hp.DiceBonus}
	return monsterItem{
		HitPoints: clamp32(max(res.Total+hp.DiceBonus, 1), 1, math.MaxInt32), Dice: expr.String(), Faces: faces, Modifier: clamp32(hp.DiceBonus, -1000, 1000),
	}, nil
}

// startMonsters is the monsters of a StartEncounter request (the ones "Começar este combate"
// sends), checked before the transaction: each an SRD creature, 1 to 40 of it (a start has no
// 10-copy limit: it is the whole encounter), a creature once, and the modes. It reads the
// creatures' hit points, so nothing reads through the pool inside the transaction.
type startMonsters struct {
	batches        []monsterBatch
	rolled, hidden bool
}

// count is how many monsters the start brings.
func (m startMonsters) count() int {
	n := 0
	for _, b := range m.batches {
		n += b.count
	}
	return n
}

func (s *Service) startMonsters(ctx context.Context, campaignID string, req *playv1.StartEncounterRequest) (startMonsters, error) {
	out := startMonsters{hidden: req.MonstersHidden == nil || req.GetMonstersHidden()}
	switch req.GetMonsterHitPoints() {
	case playv1.MonsterHitPoints_MONSTER_HIT_POINTS_UNSPECIFIED, playv1.MonsterHitPoints_MONSTER_HIT_POINTS_AVERAGE:
	case playv1.MonsterHitPoints_MONSTER_HIT_POINTS_ROLLED:
		out.rolled = true
	default:
		return startMonsters{}, connect.NewError(connect.CodeInvalidArgument, errors.New("monster_hit_points is not a known mode"))
	}
	groups := req.GetMonsters()
	if len(groups) > maxEncounterEntries {
		return startMonsters{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("monsters has at most %d groups", maxEncounterEntries))
	}
	var nameOf func(string) string // read once, for the first group that needs it
	for i, g := range groups {
		count := int(g.GetCount())
		if count == 0 {
			count = 1
		}
		if count < 1 || count > maxEntryCount {
			return startMonsters{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("monsters[%d].count must be 1 to %d", i, maxEntryCount))
		}
		if slices.ContainsFunc(out.batches, func(b monsterBatch) bool { return b.creatureKey == g.GetCreatureKey() }) {
			return startMonsters{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("monsters[%d] repeats a creature", i))
		}
		hp, ok, err := s.roster.MonsterHitPoints(ctx, nil, campaignID, g.GetCreatureKey())
		if err != nil {
			return startMonsters{}, s.dbError(ctx, "read a creature", err)
		}
		if !ok {
			return startMonsters{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("monsters[%d].creature_key is not an SRD creature", i))
		}
		if nameOf == nil && strings.TrimSpace(g.GetName()) == "" {
			if nameOf, err = s.roster.ContentNames(ctx, nil, campaignID); err != nil {
				return startMonsters{}, s.dbError(ctx, "read the content names", err)
			}
		}
		name, err := s.monsterName(ctx, campaignID, g.GetCreatureKey(), g.GetName(), nameOf)
		if err != nil {
			return startMonsters{}, err
		}
		out.batches = append(out.batches, monsterBatch{creatureKey: g.GetCreatureKey(), name: name, count: count, hp: hp})
	}
	return out, nil
}

// countPlanned is how many combatants the participants make.
func countPlanned(parts []planned) int {
	n := 0
	for _, p := range parts {
		n += p.count
	}
	return n
}

// joinStart puts the participants and the start's monsters into the new combat c.enc, inside the
// start's transaction, and returns every combatant added. The monsters go through the same
// planMonsters as AddMonsters; the master's line of their hit points ("Bandido 1: 9 PV") is
// written as AddMonsters writes it (a combatants_added event with the monsters, in as many
// events as the payload limit asks), with no idempotency key of its own: the start's key
// covers the whole.
func (s *Service) joinStart(ctx context.Context, c *combatTx, m authz.Membership, grid link.Grid, parts []planned, mon startMonsters) ([]playdb.Combatant, error) {
	before := countPlanned(parts) // the combatants ahead of the monsters in the order of insertion
	var items [][]monsterItem
	if len(mon.batches) > 0 {
		mparts, batchItems, err := s.planMonsters(ctx, c, m, mon.batches, mon.rolled, mon.hidden)
		if err != nil {
			return nil, err
		}
		parts, items = append(slices.Clone(parts), mparts...), batchItems
	}
	_, added, err := s.addParticipants(ctx, c, grid, nil, parts)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return added, nil
	}
	var all []monsterItem
	for _, batch := range items {
		for _, it := range batch {
			it.ID = added[before+len(all)].ID
			all = append(all, it)
		}
	}
	// The master's line lists each monster with its dice and faces, which is more than
	// one event's payload holds for a big start: it is written as several lines, each
	// with as many monsters as fit.
	for at := 0; at < len(all); {
		n := fitPrefix(len(all)-at, func(count int) bool {
			body, err := json.Marshal(actionEvent{Round: c.enc.Round, Monsters: &monstersEvent{Rolled: mon.rolled, Hidden: mon.hidden, Count: count, Items: all[at : at+count]}})
			return err == nil && len(body) <= eventPayloadBudget
		})
		ev := monstersEvent{Rolled: mon.rolled, Hidden: mon.hidden, Count: n, Items: all[at : at+n]}
		if err := insertEvent(ctx, c, eventCombatantsAdded, &m.UserID, nil, actionEvent{Round: c.enc.Round, Monsters: &ev}); err != nil {
			return nil, err
		}
		at += n
	}
	return added, nil
}

// partyCreatureCount is how many live creatures the player characters of the start bring with them
// (a familiar, summoned animals): they join the combat with their owners and take room in the 40.
// Only counted when the start has monsters, the one case where the count can be passed.
func (s *Service) partyCreatureCount(ctx context.Context, campaignID string, parts []planned, mon startMonsters) (int, error) {
	if len(mon.batches) == 0 {
		return 0, nil
	}
	ids := playerCharacterIDs(parts)
	if len(ids) == 0 {
		return 0, nil
	}
	creatures, err := s.roster.CharacterCreatures(ctx, nil, campaignID, ids)
	if err != nil {
		return 0, s.dbError(ctx, "read the party's creatures", err)
	}
	return len(creatures), nil
}
