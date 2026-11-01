package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// Who joins a combat: the participants of a request, checked (participants), and their
// combatants, written in the combat's transaction (addParticipants).

// planned is a participant of a combat once checked: the character, how many
// copies fight, and whether they start hidden.
type planned struct {
	char   link.Character
	count  int
	hidden bool
	// A monster of the bestiary (RN-29) is a copy of its creature's NPC with its own
	// name and hit points: name is the base of the copies' labels (the master's, or
	// the creature's), and hitPoints each copy's maximum, in order.
	name      string
	hitPoints []int
}

// participants checks who joins a combat and reads their characters. With
// withParty, a request without any player character brings the whole party:
// a combat of nothing but NPCs makes no sense. emptyOK says an empty list is fine
// (a start with monsters: the party joins).
func (s *Service) participants(ctx context.Context, campaignID string, in []*playv1.Participant, withParty, emptyOK bool) ([]planned, error) {
	ids, err := participantIDs(in, emptyOK)
	if err != nil {
		return nil, err
	}
	out, err := s.plannedParticipants(ctx, campaignID, in, ids)
	if err != nil {
		return nil, err
	}
	if withParty && !slices.ContainsFunc(out, func(p planned) bool { return p.char.Player }) {
		if out, err = s.withParty(ctx, campaignID, out); err != nil {
			return nil, err
		}
	}
	// The party first, then the NPCs, each in the order given.
	slices.SortStableFunc(out, func(a, b planned) int {
		switch {
		case a.char.Player == b.char.Player:
			return 0
		case a.char.Player:
			return -1
		}
		return 1
	})
	if countPlanned(out) > maxCombatants {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("a combat has at most %d combatants", maxCombatants))
	}
	return out, nil
}

// participantIDs reads the characters' ids of a request: how many there may be, that each is
// an id and none repeats.
func participantIDs(in []*playv1.Participant, emptyOK bool) ([]string, error) {
	if len(in) == 0 && !emptyOK {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("participants must list who fights"))
	}
	if len(in) > maxCombatants {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("participants must have at most %d entries", maxCombatants))
	}
	ids := make([]string, 0, len(in))
	for i, p := range in {
		id, ok := uid.Canonical(p.GetCharacterId())
		if !ok {
			return nil, errCharacterNotFound()
		}
		if slices.Contains(ids, id) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("participants[%d] repeats a character", i))
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// plannedParticipants reads the characters of the request and checks each participant.
func (s *Service) plannedParticipants(ctx context.Context, campaignID string, in []*playv1.Participant, ids []string) ([]planned, error) {
	found, err := s.roster.CombatCharacters(ctx, nil, campaignID, ids)
	if err != nil {
		return nil, s.dbError(ctx, "read the characters of a combat", err)
	}
	byID := make(map[string]link.Character, len(found))
	for _, c := range found {
		byID[c.ID] = c
	}
	out := make([]planned, 0, len(in))
	for i, p := range in {
		c, ok := byID[ids[i]]
		if !ok {
			if err := s.refuseReserved(ctx, campaignID, ids[i]); err != nil {
				return nil, err
			}
			return nil, errCharacterNotFound()
		}
		one, err := planParticipant(c, p, i)
		if err != nil {
			return nil, err
		}
		out = append(out, one)
	}
	return out, nil
}

// planParticipant checks one participant against its character: the count and the hiding.
func planParticipant(c link.Character, p *playv1.Participant, i int) (planned, error) {
	if c.CombatOnly {
		// The NPC the app keeps for a creature's monsters is AddMonsters's.
		return planned{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("participants[%d] is not a character the master can pick: use AddMonsters", i))
	}
	count := int(p.GetCount())
	if count == 0 {
		count = 1
	}
	hidden := !c.Player // a new NPC starts hidden (question 31)
	if p.Hidden != nil {
		hidden = p.GetHidden()
	}
	switch {
	case c.Player && (count != 1 || hidden):
		return planned{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("participants[%d]: a player's character joins once, and is never hidden", i))
	case count < 1 || count > maxNPCCopies:
		return planned{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("participants[%d].count must be 1 to %d", i, maxNPCCopies))
	}
	return planned{char: c, count: count, hidden: hidden}, nil
}

// withParty adds the whole party to a request that names no player character.
func (s *Service) withParty(ctx context.Context, campaignID string, out []planned) ([]planned, error) {
	party, err := s.roster.CombatParty(ctx, nil, campaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read the party", err)
	}
	for _, c := range party {
		out = append(out, planned{char: c, count: 1})
	}
	return out, nil
}

// refuseReserved turns the "not found" of a reserved character (imported, not claimed by a player
// yet: it cannot fight) into a failed_precondition that says so, not a missing character.
func (s *Service) refuseReserved(ctx context.Context, campaignID, characterID string) error {
	found, err := s.roster.SessionCharacters(ctx, nil, campaignID, []string{characterID})
	if err != nil {
		return s.dbError(ctx, "read a character of a combat", err)
	}
	if len(found) == 1 && found[0].Reserved {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_CHARACTER_RESERVED,
			"a reserved character (no player yet) cannot fight")
	}
	return nil
}

func errCharacterNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("character not found"))
}

// joining is a join in progress: the combat's squares and tokens, the labels taken, and the
// combatants there are and the ones added.
type joining struct {
	grid   link.Grid
	tokens []link.TokenPosition
	taken  map[string]bool
	all    []playdb.Combatant
	added  []playdb.Combatant
}

// addParticipants inserts the combatants of the participants into the combat
// c.enc, rolls each NPC's own initiative (RN-19), puts them in the turn
// order, and returns the whole order and the combatants it added. A
// combatant starts on the square of its character's token, when it has one
// and nobody stands there (an NPC with several copies puts only the first
// one there). existing are
// the combatants already in the combat.
func (s *Service) addParticipants(ctx context.Context, c *combatTx, grid link.Grid, existing []playdb.Combatant, parts []planned) (order, added []playdb.Combatant, err error) {
	j := joining{grid: grid, taken: make(map[string]bool, len(existing)), all: slices.Clone(existing)}
	if c.enc.MapID != nil {
		if j.tokens, err = s.maps.MapTokens(ctx, c.tx, *c.enc.MapID); err != nil {
			return nil, nil, fmt.Errorf("read the tokens: %w", err)
		}
	}
	for _, e := range existing {
		j.taken[e.Label] = true
	}
	for _, p := range parts {
		if err := s.joinParticipant(ctx, c, &j, p); err != nil {
			return nil, nil, err
		}
	}
	// A player's character brings its creatures (MR-037): the ones it has when the
	// combat is set up. Each has no initiative yet; its owner's player rolls one
	// for the group, and the master can take any of them out.
	if err := s.joinOwnCreatures(ctx, c, &j, parts); err != nil {
		return nil, nil, err
	}
	return s.settleJoined(ctx, c, &j)
}

// participantLabels names the copies of a participant: a player by its character, an NPC by
// the name of its monster when it has one.
func participantLabels(p planned, taken map[string]bool) []string {
	switch {
	case p.char.Player:
		return copyLabels(p.char.Name, 1, taken)
	case p.name != "":
		return copyLabels(p.name, p.count, taken)
	}
	return copyLabels(p.char.Name, p.count, taken)
}

// joinParticipant inserts the copies of one participant.
func (s *Service) joinParticipant(ctx context.Context, c *combatTx, j *joining, p planned) error {
	for i, label := range participantLabels(p, j.taken) {
		row, err := s.joinRow(c, j, p, i, label)
		if err != nil {
			return err
		}
		inserted, err := s.insertJoined(ctx, c, row, p)
		if err != nil {
			return err
		}
		j.all = append(j.all, inserted)
		j.added = append(j.added, inserted)
	}
	return nil
}

// joinRow is the row of the i-th copy of a participant: copied from the sheet, rolled for an
// NPC, and placed on its token's square when nobody stands there.
func (s *Service) joinRow(c *combatTx, j *joining, p planned, i int, label string) (playdb.InsertCombatantParams, error) {
	row := playdb.InsertCombatantParams{
		EncounterID: c.enc.ID, CharacterID: p.char.ID, Label: label, Kind: kindNPC, Hidden: p.hidden,
		InitiativeBonus: clamp32(p.char.InitiativeBonus, -20, 40), OrderIndex: clamp32(len(j.all), 0, math.MaxInt32),
		SpeedFt: clamp32(p.char.SpeedFt, 0, 600), CreatedAt: c.now,
		// Copied from the sheet, like the speed: the side, size and fly speed
		// that decide who it can pass, and the jump limits (MR-034, RN-21).
		Side: sideOfKind(kindNPC), Size: sizeKey(p.char.Size), SpeedFlyFt: clamp32(p.char.SpeedFlyFt, 0, 600),
		JumpLongDft: clamp32(p.char.JumpLongDFt, 0, 6000), JumpHighDft: clamp32(p.char.JumpHighDFt, 0, 6000),
		JumpLongBonusDft: clamp32(p.char.JumpLongBonusDFt, 0, 6000),
	}
	if p.char.Player {
		playerRow(&row, p.char)
	} else if err := s.npcRow(&row, p, i); err != nil {
		return row, err
	}
	// Never on a square someone already stands on (reinforcements of
	// an NPC already in the fight): the master places that one.
	if i == 0 {
		placeOnToken(&row, j, p.char.ID)
	}
	return row, nil
}

// playerRow makes the row a player's combatant.
func playerRow(row *playdb.InsertCombatantParams, char link.Character) {
	row.Kind, row.Side = kindPlayer, sideOfKind(kindPlayer)
	if char.PlayerUserID != "" {
		row.UserID = &char.PlayerUserID
	}
}

// npcRow gives an NPC's row its hit points and XP, and rolls its initiative: each copy rolls
// for itself, in the app (RN-19).
func (s *Service) npcRow(row *playdb.InsertCombatantParams, p planned, i int) error {
	hp := clamp32(p.char.HitPointsMax, 1, math.MaxInt32)
	if i < len(p.hitPoints) {
		hp = clamp32(p.hitPoints[i], 1, math.MaxInt32)
	}
	row.HpCurrent, row.HpMax, row.HpTemp = &hp, &hp, new(int32(0))
	row.XpValue = clamp32(p.char.XPValue, 0, 1_000_000) // MR-016: what it gives when defeated
	face, total, err := s.rollInitiative(int(row.InitiativeBonus))
	if err != nil {
		return err
	}
	row.Initiative, row.InitiativeFace = new(conv.Int32(total)), new(clamp32(face, 1, 20))
	return nil
}

// placeOnToken puts the row on the square of its character's token, when it has one and nobody
// stands there.
func placeOnToken(row *playdb.InsertCombatantParams, j *joining, characterID string) {
	t, ok := tokenOf(j.tokens, characterID)
	if !ok {
		return
	}
	col, r := squareOf(j.grid, t.XBP, t.YBP)
	if !slices.ContainsFunc(j.all, func(o playdb.Combatant) bool {
		return placed(o) && *o.GridCol == col && *o.GridRow == r
	}) {
		row.GridCol, row.GridRow = &col, &r
	}
}

// insertJoined writes the row of a copy and what its joining does: a player's character leaves
// a familiar's sight, and the spells cast outside the combat go in with it.
func (s *Service) insertJoined(ctx context.Context, c *combatTx, row playdb.InsertCombatantParams, p planned) (playdb.Combatant, error) {
	inserted, err := c.q.InsertCombatant(ctx, row)
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23503" && pgErr.ConstraintName == "combatants_character_id_fkey" {
		return inserted, errCharacterNotFound() // deleted after the participants were read
	}
	if err != nil {
		return inserted, fmt.Errorf("insert combatant: %w", err)
	}
	// A hand-made NPC brings what it spent in earlier combats (npc_spell_use): slots, uses, uses a day and
	// Legendary Resistance. A monster starts with nothing.
	if !p.char.Player && !p.char.CombatOnly {
		used, err := c.q.SeedCombatantSlotsUsed(ctx, inserted.ID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return inserted, fmt.Errorf("seed the NPC's spent slots: %w", err)
		}
		if err == nil {
			inserted.SlotsUsed, inserted.CreatureSpent = used.SlotsUsed, used.CreatureSpent
		}
	}
	// A character that joins a combat leaves any familiar's sight it had: the
	// sight is an action of a turn there (MR-036).
	if p.char.Player {
		if err := s.endSight(ctx, c, inserted, sightCombatJoined, false); err != nil {
			return inserted, err
		}
	}
	// The spells cast outside the combat go in with it (casting_combat.go).
	if err := s.carryCasts(ctx, c, inserted); err != nil {
		return inserted, err
	}
	return inserted, nil
}

// joinOwnCreatures brings the creatures of the player characters that joined.
func (s *Service) joinOwnCreatures(ctx context.Context, c *combatTx, j *joining, parts []planned) error {
	ids := playerCharacterIDs(parts)
	if len(ids) == 0 {
		return nil
	}
	creatures, err := s.roster.CharacterCreatures(ctx, c.tx, c.session.CampaignID, ids)
	if err != nil {
		return err
	}
	var joined []playdb.Combatant
	if j.all, joined, err = s.joinCreatures(ctx, c, j.all, creatures, nil); err != nil {
		return err
	}
	j.added = append(j.added, joined...)
	return nil
}

// settleJoined puts everybody in the turn order and returns it with the combatants added, as
// they are now in the order.
func (s *Service) settleJoined(ctx context.Context, c *combatTx, j *joining) (order, added []playdb.Combatant, err error) {
	if err := s.effectsToCombat(ctx, c, j.all, j.added); err != nil {
		return nil, nil, err
	}
	order, err = saveOrder(ctx, c.q, j.all, orderCombatants(j.all))
	if err != nil {
		return nil, nil, err
	}
	// The added combatants as they are now in the order.
	for i, a := range j.added {
		j.added[i] = order[slices.IndexFunc(order, func(o playdb.Combatant) bool { return o.ID == a.ID })]
	}
	return order, j.added, nil
}

func tokenOf(tokens []link.TokenPosition, characterID string) (link.TokenPosition, bool) {
	i := slices.IndexFunc(tokens, func(t link.TokenPosition) bool { return t.CharacterID == characterID })
	if i < 0 {
		return link.TokenPosition{}, false
	}
	return tokens[i], true
}

// rollInitiative rolls a d20 in the app and adds the bonus (RN-19): the face
// and the total.
func (s *Service) rollInitiative(bonus int) (face, total int, err error) {
	res, err := dice.Roll(s.roller, dice.Expr{Count: 1, Sides: d20Sides, Modifier: bonus})
	if err != nil {
		return 0, 0, fmt.Errorf("roll the initiative: %w", err)
	}
	return res.Faces[0], res.Total, nil
}

// clamp32 converts n to an int32 inside [lo, hi]; the int32 range is checked
// by conv.Int32, so the conversion is safe whatever lo and hi are.
func clamp32(n, lo, hi int) int32 {
	return conv.Int32(min(max(n, lo), hi))
}
