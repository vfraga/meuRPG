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

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/proto"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The combat (MR-013, Etapa 6): CombatService, in the same Service as
// PlayService because a combat lives inside the open game session and its
// events travel on the same stream. This file has the handlers; the helpers
// are in combat_write.go (the transaction every change shares),
// combat_view.go (what each viewer sees) and combat_rules.go (the pure rules).
//
// A combat is an encounters row with combatants rows (migration 00043 and
// 00044). Every handler starts with one explicit check, and every change
// takes the open session's row lock, checks the idempotency key, changes the
// rows, writes a session event, and only then, after the commit, publishes.

// StartEncounter implements playv1connect.CombatServiceHandler.
func (s *Service) StartEncounter(
	ctx context.Context,
	req *connect.Request[playv1.StartEncounterRequest],
) (*connect.Response[playv1.StartEncounterResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	name, err := names.Clean(req.Msg.GetName(), maxEncounterName)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name %w", err))
	}
	pointID, err := optionalID(req.Msg.GetMapPointId(), "point not found")
	if err != nil {
		return nil, err
	}
	asked := req.Msg.GetMode()
	if _, known := playv1.EncounterMode_name[int32(asked)]; !known {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("mode must be a mode of combat"))
	}
	monsters, err := s.startMonsters(ctx, m.CampaignID, req.Msg)
	if err != nil {
		return nil, err
	}
	// With monsters, an empty list of participants is fine: the whole party joins.
	parts, err := s.participants(ctx, m.CampaignID, req.Msg.GetParticipants(), true, len(monsters.batches) > 0)
	if err != nil {
		return nil, err
	}
	// The party's own creatures (a familiar, summoned animals) join with their owners, so they take room too.
	kept, err := s.partyCreatureCount(ctx, m.CampaignID, parts, monsters)
	if err != nil {
		return nil, err
	}
	if total := countPlanned(parts) + monsters.count() + kept; total > maxCombatants {
		return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TOO_MANY_COMBATANTS, fmt.Sprintf("a combat has at most %d combatants", maxCombatants))
	}
	var point link.BattlePoint
	if pointID != nil {
		if point, err = s.maps.BattlePoint(ctx, m.CampaignID, *pointID); err != nil {
			return nil, s.dbError(ctx, "find the battle point", err)
		}
	}

	digest := startDigest(req.Msg)
	var newMap string // the map the point made current, if it changed
	// No request hash: the start is compared by its own digest, which reads the defaults written out as the same request.
	res, err := s.write(ctx, combatWrite{m: m, key: key, kind: eventEncounterStarted}, func(c *combatTx) (any, error) {
		newMap = ""
		_, err := c.q.GetOpenEncounter(ctx, c.session.ID)
		switch {
		case err == nil:
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ENCOUNTER_ALREADY_OPEN,
				"the session already has a combat; end it first")
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, fmt.Errorf("find the open encounter: %w", err)
		}

		// The mode is chosen here, for good (RN-25): the request's, or the table's rule
		// "combate com mapa" read in this transaction.
		if asked == playv1.EncounterMode_ENCOUNTER_MODE_UNSPECIFIED && pointID != nil {
			asked = playv1.EncounterMode_ENCOUNTER_MODE_GRID // a battle point leads to a map: it implies a combat on one, whatever the table's rule
		}
		mode, err := s.modeOfStart(ctx, c.tx, m.CampaignID, asked)
		if err != nil {
			return nil, err
		}
		if mode == modeTheatre {
			// No map, no point, no grid: nobody has a square, and the session's current
			// map stays as it is.
			if pointID != nil {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_THEATRE_HAS_NO_MAP,
					"a combat without a map takes no battle point")
			}
			enc, err := c.q.InsertEncounter(ctx, playdb.InsertEncounterParams{
				GameSessionID: c.session.ID, Name: name, CreatedAt: c.now, Mode: modeTheatre,
			})
			if err != nil {
				return nil, fmt.Errorf("insert encounter: %w", err)
			}
			c.enc = enc
			added, err := s.joinStart(ctx, c, m, link.Grid{}, parts, monsters)
			if err != nil {
				return nil, err
			}
			return map[string]any{"encounter_id": enc.ID, "combatants": len(added), "mode": modeTheatre, "digest": digest}, nil
		}

		// The fight is on the session's current map, or on the map the battle
		// point leads to, which then becomes the current one.
		mapID := deref(c.session.CurrentMapID)
		if point.TargetMapID != "" {
			mapID = point.TargetMapID
		}
		if mapID == "" {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NO_CURRENT_MAP,
				"the session has no current map to fight on")
		}
		grid, err := s.maps.MapGrid(ctx, c.tx, m.CampaignID, mapID)
		if err != nil {
			return nil, err
		}
		if !grid.OK() {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_MAP_HAS_NO_GRID,
				"the map has no grid", func(b *playv1.EncounterBlocked) { b.MapId = mapID })
		}
		if mapID != deref(c.session.CurrentMapID) {
			// As SetCurrentMap: the players see the current map, so it is revealed.
			if err := s.maps.RevealMap(ctx, c.tx, m.CampaignID, mapID, c.now); err != nil {
				return nil, err
			}
			if _, err := c.q.SetCurrentMap(ctx, playdb.SetCurrentMapParams{ID: c.session.ID, CurrentMapID: &mapID}); err != nil {
				return nil, fmt.Errorf("set the current map: %w", err)
			}
			newMap = mapID
		}

		enc, err := c.q.InsertEncounter(ctx, playdb.InsertEncounterParams{
			GameSessionID: c.session.ID, MapID: &mapID, MapPointID: pointID, Name: name,
			GridColumns: grid.Columns, GridRows: grid.Rows, CreatedAt: c.now, Mode: modeGrid,
		})
		if err != nil {
			return nil, fmt.Errorf("insert encounter: %w", err)
		}
		c.enc = enc
		added, err := s.joinStart(ctx, c, m, grid, parts, monsters)
		if err != nil {
			return nil, err
		}
		return map[string]any{"encounter_id": enc.ID, "combatants": len(added), "map_id": mapID, "digest": digest}, nil
	})
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" && pgErr.ConstraintName == "encounters_one_open_per_session" {
		// Another start of the same session won the race.
		err = errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ENCOUNTER_ALREADY_OPEN,
			"the session already has a combat; end it first")
	}
	if err != nil {
		return nil, s.dbError(ctx, "start an encounter", err)
	}
	if res.repeated && !sameStart(res.payload, digest) {
		// A retry answers with the first start only when it asks for the same one.
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
	}
	if newMap != "" {
		s.maps.MapShown(ctx, m.CampaignID, newMap)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		if newMap != "" {
			s.Publish(m.CampaignID, true, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_CurrentMapChanged_{
				CurrentMapChanged: &playv1.WatchGameSessionResponse_CurrentMapChanged{MapId: newMap},
			}})
		}
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.StartEncounterResponse{Encounter: out}), nil
}

// startDigest identifies what a StartEncounter asks for (every field but the idempotency key), so a
// retry with the same key and another request is told apart from a true retry. The event of the
// start keeps it; the monsters, their modes, the participants, the mode, the point and the name
// are all in it.
func startDigest(req *playv1.StartEncounterRequest) string {
	c := proto.Clone(req).(*playv1.StartEncounterRequest)
	c.IdempotencyKey = ""
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(c)
	if err != nil {
		return "" // never: a message of this package
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:12])
}

// sameStart says whether the stored event of a start was made by a request with this digest. An
// event written before the digest existed has none and is taken as the same.
func sameStart(payload []byte, digest string) bool {
	var ev struct {
		Digest string `json:"digest"`
	}
	if json.Unmarshal(payload, &ev) != nil || ev.Digest == "" {
		return true
	}
	return ev.Digest == digest
}

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
	if len(in) == 0 && !emptyOK {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("participants must list who fights"))
	}
	if len(in) > maxCombatants {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("participants must have at most %d entries", maxCombatants))
	}
	ids := make([]string, 0, len(in))
	for i, p := range in {
		id, ok := parseID(p.GetCharacterId())
		if !ok {
			return nil, errCharacterNotFound()
		}
		if slices.Contains(ids, id) {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("participants[%d] repeats a character", i))
		}
		ids = append(ids, id)
	}
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
			return nil, errCharacterNotFound()
		}
		if c.CombatOnly {
			// The NPC the app keeps for a creature's monsters is AddMonsters's.
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("participants[%d] is not a character the master can pick: use AddMonsters", i))
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
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("participants[%d]: a player's character joins once, and is never hidden", i))
		case count < 1 || count > maxNPCCopies:
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("participants[%d].count must be 1 to %d", i, maxNPCCopies))
		}
		out = append(out, planned{char: c, count: count, hidden: hidden})
	}
	if withParty && !slices.ContainsFunc(out, func(p planned) bool { return p.char.Player }) {
		party, err := s.roster.CombatParty(ctx, nil, campaignID)
		if err != nil {
			return nil, s.dbError(ctx, "read the party", err)
		}
		for _, c := range party {
			out = append(out, planned{char: c, count: 1})
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
	total := 0
	for _, p := range out {
		total += p.count
	}
	if total > maxCombatants {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("a combat has at most %d combatants", maxCombatants))
	}
	return out, nil
}

func errCharacterNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("character not found"))
}

func parseID(raw string) (string, bool) {
	id, err := parseCombatID(raw, "")
	return id, err == nil
}

// addParticipants inserts the combatants of the participants into the combat
// c.enc, rolls each NPC's own initiative (RN-19), puts them in the turn
// order, and returns the whole order and the combatants it added. A
// combatant starts on the square of its character's token, when it has one
// and nobody stands there (an NPC with several copies puts only the first
// one there). existing are
// the combatants already in the combat.
func (s *Service) addParticipants(ctx context.Context, c *combatTx, grid link.Grid, existing []playdb.Combatant, parts []planned) (order, added []playdb.Combatant, err error) {
	var tokens []link.TokenPosition
	if c.enc.MapID != nil {
		if tokens, err = s.maps.MapTokens(ctx, c.tx, *c.enc.MapID); err != nil {
			return nil, nil, fmt.Errorf("read the tokens: %w", err)
		}
	}
	taken := make(map[string]bool, len(existing))
	for _, e := range existing {
		taken[e.Label] = true
	}
	all := slices.Clone(existing)
	for _, p := range parts {
		var labels []string
		switch {
		case p.char.Player:
			labels = copyLabels(p.char.Name, 1, taken)
		case p.name != "":
			labels = copyLabels(p.name, p.count, taken)
		default:
			labels = copyLabels(p.char.Name, p.count, taken)
		}
		for i, label := range labels {
			row := playdb.InsertCombatantParams{
				EncounterID: c.enc.ID, CharacterID: p.char.ID, Label: label, Kind: kindNPC, Hidden: p.hidden,
				InitiativeBonus: clamp32(p.char.InitiativeBonus, -20, 40), OrderIndex: clamp32(len(all), 0, math.MaxInt32),
				SpeedFt: clamp32(p.char.SpeedFt, 0, 600), CreatedAt: c.now,
				// Copied from the sheet, like the speed: the side, size and fly speed
				// that decide who it can pass, and the jump limits (MR-034, RN-21).
				Side: sideOfKind(kindNPC), Size: sizeKey(p.char.Size), SpeedFlyFt: clamp32(p.char.SpeedFlyFt, 0, 600),
				JumpLongDft: clamp32(p.char.JumpLongDFt, 0, 6000), JumpHighDft: clamp32(p.char.JumpHighDFt, 0, 6000),
			}
			if p.char.Player {
				row.Kind, row.Side = kindPlayer, sideOfKind(kindPlayer)
				if p.char.PlayerUserID != "" {
					row.UserID = &p.char.PlayerUserID
				}
			} else {
				hp := clamp32(p.char.HitPointsMax, 1, math.MaxInt32)
				if i < len(p.hitPoints) {
					hp = clamp32(p.hitPoints[i], 1, math.MaxInt32)
				}
				row.HpCurrent, row.HpMax, row.HpTemp = &hp, &hp, new(int32(0))
				row.XpValue = clamp32(p.char.XPValue, 0, 1_000_000) // MR-016: what it gives when defeated
				// Each copy rolls for itself, in the app (RN-19).
				face, total, err := s.rollInitiative(int(row.InitiativeBonus))
				if err != nil {
					return nil, nil, err
				}
				row.Initiative, row.InitiativeFace = new(clamp32(total, math.MinInt32, math.MaxInt32)), new(clamp32(face, 1, 20))
			}
			// Never on a square someone already stands on (reinforcements of
			// an NPC already in the fight): the master places that one.
			if i == 0 {
				if t, ok := tokenOf(tokens, p.char.ID); ok {
					col, r := squareOf(grid, t.XBP, t.YBP)
					if !slices.ContainsFunc(all, func(o playdb.Combatant) bool {
						return placed(o) && *o.GridCol == col && *o.GridRow == r
					}) {
						row.GridCol, row.GridRow = &col, &r
					}
				}
			}
			inserted, err := c.q.InsertCombatant(ctx, row)
			if err != nil {
				return nil, nil, fmt.Errorf("insert combatant: %w", err)
			}
			// A character that joins a combat leaves any familiar's sight it had: the
			// sight is an action of a turn there (MR-036).
			if p.char.Player {
				if err := s.endSight(ctx, c, inserted, sightCombatJoined, false); err != nil {
					return nil, nil, err
				}
			}
			all = append(all, inserted)
			added = append(added, inserted)
		}
	}
	// A player's character brings its creatures (MR-037): the ones it has when the
	// combat is set up. Each has no initiative yet; its owner's player rolls one
	// for the group, and the master can take any of them out.
	if ids := playerCharacterIDs(parts); len(ids) > 0 {
		creatures, err := s.roster.CharacterCreatures(ctx, c.tx, c.session.CampaignID, ids)
		if err != nil {
			return nil, nil, err
		}
		var joined []playdb.Combatant
		if all, joined, err = s.joinCreatures(ctx, c, all, creatures, nil); err != nil {
			return nil, nil, err
		}
		added = append(added, joined...)
	}
	order, err = saveOrder(ctx, c.q, all, orderCombatants(all))
	if err != nil {
		return nil, nil, err
	}
	// The added combatants as they are now in the order.
	for i, a := range added {
		added[i] = order[slices.IndexFunc(order, func(o playdb.Combatant) bool { return o.ID == a.ID })]
	}
	return order, added, nil
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
	res, err := dice.Roll(s.roller, dice.Expr{Count: 1, Sides: 20, Modifier: bonus})
	if err != nil {
		return 0, 0, fmt.Errorf("roll the initiative: %w", err)
	}
	return res.Faces[0], res.Total, nil
}

// clamp32 converts n to an int32 inside [lo, hi].
func clamp32(n, lo, hi int) int32 {
	return int32(min(max(n, lo), hi, math.MaxInt32)) //nolint:gosec // clamped to the int32 range just above
}

// SubmitInitiative implements playv1connect.CombatServiceHandler.
func (s *Service) SubmitInitiative(
	ctx context.Context,
	req *connect.Request[playv1.SubmitInitiativeRequest],
) (*connect.Response[playv1.SubmitInitiativeResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
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
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	var inApp bool
	var face int
	switch roll := req.Msg.GetRoll().(type) {
	case *playv1.SubmitInitiativeRequest_RollInApp:
		if !roll.RollInApp {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		inApp = true
	case *playv1.SubmitInitiativeRequest_D20Face:
		face = int(roll.D20Face)
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or d20_face"))
	}
	v := viewerOf(m)
	// RN-18: only a mode the master forced binds a player; with "each player
	// chooses" they pick on every roll. The master rolls either way, for anyone.
	var force DiceForce
	if !v.master {
		if force, err = s.dice.ForcedDice(ctx, nil, m.CampaignID, m.UserID); err != nil {
			return nil, s.dbError(ctx, "read the dice setting", err)
		}
	}

	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventInitiativeSubmitted, encounterID: encID}, func(c *combatTx) (any, error) {
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		v = c.viewer(m, cs) // the fog: an NPC the player does not see is not found
		target, err := findCombatant(cs, combID, v)
		if err != nil {
			return nil, err
		}
		if err := v.mayAct(target); err != nil {
			return nil, err
		}
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		if c.enc.Status != statusSetup {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_IN_SETUP, "initiative is rolled before the combat begins")
		}
		if !v.master {
			if target.Initiative != nil {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_INITIATIVE_ALREADY_SET, "your initiative is already set")
			}
			if force.refuses(inApp) {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_WRONG_DICE_MODE, "this is not how the campaign has you roll your dice")
			}
		}
		expr := dice.Expr{Count: 1, Sides: 20, Modifier: int(target.InitiativeBonus)}
		var roll dice.Result
		if inApp {
			roll, err = dice.Roll(s.roller, expr)
			if err != nil {
				return nil, fmt.Errorf("roll the initiative: %w", err)
			}
			face = roll.Faces[0]
		} else if roll, err = dice.Physical(expr, face); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be 1 to 20"))
		}
		total, f := clamp32(roll.Total, math.MinInt32, math.MaxInt32), clamp32(face, 1, 20)
		if isCreature(target) {
			// The creatures of one casting roll once (RN-18): every member of the group
			// takes the roll, with the bonus of the one that rolled, so they take a
			// joint turn.
			if err := c.q.SetGroupInitiative(ctx, playdb.SetGroupInitiativeParams{
				EncounterID: c.enc.ID, SummonGroupID: deref(target.SummonGroupID), Initiative: &total, InitiativeFace: &f, InitiativeBonus: target.InitiativeBonus,
			}); err != nil {
				return nil, fmt.Errorf("save the group's initiative: %w", err)
			}
			for i := range cs {
				if isCreature(cs[i]) && deref(cs[i].SummonGroupID) == deref(target.SummonGroupID) {
					cs[i].Initiative, cs[i].InitiativeFace, cs[i].InitiativeBonus, cs[i].TieOrdered = &total, &f, target.InitiativeBonus, false
				}
			}
		} else {
			if err := c.q.SetCombatantInitiative(ctx, playdb.SetCombatantInitiativeParams{ID: target.ID, Initiative: &total, InitiativeFace: &f}); err != nil {
				return nil, fmt.Errorf("save the initiative: %w", err)
			}
			for i := range cs {
				if cs[i].ID == target.ID {
					cs[i].Initiative, cs[i].InitiativeFace, cs[i].TieOrdered = &total, &f, false
				}
			}
		}
		if _, err := saveOrder(ctx, c.q, cs, orderCombatants(cs)); err != nil {
			return nil, err
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &target.CharacterID
		return map[string]any{"combatant_id": target.ID, "face": f, "total": total, "physical": !inApp}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "submit an initiative", err)
	}
	out, err := s.finish(ctx, m, res, s.changed(m.CampaignID))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SubmitInitiativeResponse{Encounter: out}), nil
}

// SetInitiativeOrder implements playv1connect.CombatServiceHandler.
func (s *Service) SetInitiativeOrder(
	ctx context.Context,
	req *connect.Request[playv1.SetInitiativeOrderRequest],
) (*connect.Response[playv1.SetInitiativeOrderResponse], error) {
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
	ids := make([]string, 0, len(req.Msg.GetCombatantIds()))
	for _, raw := range req.Msg.GetCombatantIds() {
		id, err := parseCombatID(raw, "combatant")
		if err != nil {
			return nil, err
		}
		if slices.Contains(ids, id) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("combatant_ids repeats a combatant"))
		}
		ids = append(ids, id)
	}
	if len(ids) < 2 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("combatant_ids must name at least two combatants"))
	}

	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventInitiativeOrderSet, encounterID: encID}, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		var first playdb.Combatant
		for i, id := range ids {
			found, err := findCombatant(cs, id, viewerOf(m))
			if err != nil {
				return nil, err
			}
			if i == 0 {
				first = found
			}
			if !sameTie(first, found) {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("combatant_ids must be tied: the same initiative total and bonus"))
			}
		}
		if _, err := saveOrder(ctx, c.q, cs, withOrder(orderCombatants(cs), ids)); err != nil {
			return nil, err
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		return map[string]any{"combatant_ids": ids}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "set the initiative order", err)
	}
	out, err := s.finish(ctx, m, res, s.changed(m.CampaignID))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetInitiativeOrderResponse{Encounter: out}), nil
}

// BeginCombat implements playv1connect.CombatServiceHandler.
func (s *Service) BeginCombat(
	ctx context.Context,
	req *connect.Request[playv1.BeginCombatRequest],
) (*connect.Response[playv1.BeginCombatResponse], error) {
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

	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventCombatBegun, encounterID: encID}, func(c *combatTx) (any, error) {
		if c.enc.Status != statusSetup {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_IN_SETUP, "the combat already began or ended")
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		if len(cs) == 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("the combat has no combatants"))
		}
		var missing []string
		var labels []string
		for _, c := range cs {
			if c.Initiative == nil {
				missing, labels = append(missing, c.ID), append(labels, c.Label)
			}
		}
		if len(missing) > 0 {
			// Only the master calls this, so the message may name who is missing.
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_INITIATIVE_MISSING,
				fmt.Sprintf("missing the initiative of: %v", labels), func(b *playv1.EncounterBlocked) { b.CombatantIds = missing })
		}
		// The first group starts its turn: every living member acts at once.
		first, _, ok := nextTurnGroup(cs, "", "")
		if !ok {
			first = []string{cs[0].ID} // everybody defeated: the first of the order starts
		}
		started := c.now
		c.enc.StartedAt = &started
		// No sight carries into the fight: it is an action of a turn there (MR-036).
		for _, member := range cs {
			if err := s.endSight(ctx, c, member, sightCombatJoined, false); err != nil {
				return nil, err
			}
		}
		if err := startTurn(ctx, c, first, 1); err != nil {
			return nil, err
		}
		return map[string]any{"round": 1, "current_combatant_id": first[0], "turn_group_ids": first}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "begin a combat", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishTurnChanged(ctx, m.CampaignID, d)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, true)
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.BeginCombatResponse{Encounter: out}), nil
}

// EndTurn implements playv1connect.CombatServiceHandler.
func (s *Service) EndTurn(
	ctx context.Context,
	req *connect.Request[playv1.EndTurnRequest],
) (*connect.Response[playv1.EndTurnResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
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
	// Empty only when nobody is acting (see the orphaned turn below).
	expected := ""
	if raw := req.Msg.GetExpectedCombatantId(); raw != "" {
		if expected, err = parseCombatID(raw, "combatant"); err != nil {
			return nil, err
		}
	}
	v := viewerOf(m)
	stale := connect.NewError(connect.CodeAborted, errors.New("that combatant's part of the turn is not the one running"))

	var dropped []playdb.PendingDamage // the damage the master passed the turn over
	var passed bool                    // the turn passed to the next group (the last part ended)
	var secret bool                    // the part that ended is a hidden member's: no line for the players
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventTurnEnded, altKind: eventTurnPartEnded, encounterID: encID}, func(c *combatTx) (any, error) {
		dropped, passed, secret = nil, false, false
		if c.enc.Status != statusActive {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "the combat is not running")
		}
		// A late tap from a screen that saw an earlier round: when a group is the
		// only one, the last part ending starts the next round with the same members.
		if r := req.Msg.GetExpectedRound(); r != 0 && r != c.enc.Round {
			return nil, stale
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		// A combatant the player does not see is not found, before anything else can
		// answer differently (a stale turn, a part that ended): so it is on every call.
		v = c.viewer(m, cs)
		if !v.master && expected != "" && slices.ContainsFunc(cs, func(o playdb.Combatant) bool { return o.ID == expected }) {
			if _, err := findCombatant(cs, expected, v); err != nil {
				return nil, err
			}
		}
		// An orphaned turn: nobody acts, because the combatants of the turn left
		// the fight (the last one who could act was removed, or its character was
		// deleted). The master starts the turns again from the top of the order,
		// in the same round; without this the combat could never move.
		if !slices.ContainsFunc(cs, func(o playdb.Combatant) bool { return o.TurnState == turnActing }) {
			if !v.master {
				return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only the master can start the turns again"))
			}
			// The master's screen may still hold the id of the one who left.
			if expected != "" && (c.enc.CurrentCombatantID == nil || *c.enc.CurrentCombatantID != expected) {
				return nil, stale
			}
			first, _, ok := nextTurnGroup(cs, "", "")
			if !ok {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "no combatant can take a turn")
			}
			passed = true
			if err := startTurn(ctx, c, first, c.enc.Round); err != nil {
				return nil, err
			}
			return map[string]any{"to": first[0], "round": c.enc.Round}, nil
		}
		// A double tap, or a stale screen: that member's part ended already, or
		// the turn passed, and nothing changes, so one tap never ends two parts
		// or two turns.
		at := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.ID == expected })
		if at < 0 || cs[at].TurnState != turnActing {
			return nil, stale
		}
		current, err := findCombatant(cs, expected, v)
		if err != nil {
			return nil, err
		}
		if err := v.mayAct(current); err != nil {
			return nil, err
		}
		// The turn waits for an opportunity attack's answer; the master may end it
		// anyway, which passes the offers over.
		if err := s.mustNotWait(ctx, c, current); err != nil {
			return nil, err
		}
		if _, err := c.q.SkipPendingOpportunityOffersOfMover(ctx, playdb.SkipPendingOpportunityOffersOfMoverParams{EncounterID: c.enc.ID, MoverID: current.ID, AnsweredAt: &c.now}); err != nil {
			return nil, fmt.Errorf("pass the opportunity offers over: %w", err)
		}
		// A player whose character is down rolls its death save first (RN-03); the
		// master may end the part anyway.
		if !v.master && current.Kind == kindPlayer {
			vit, err := s.vitals.GetVitalsTx(ctx, c.tx, c.session.CampaignID, current.CharacterID)
			if err != nil && connect.CodeOf(err) != connect.CodeNotFound { // not found: the character died meanwhile
				return nil, err
			}
			if err == nil && deathSaveDue(current, vit) {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_DEATH_SAVE_DUE, "roll your death save before ending the turn")
			}
		}
		// A damage still to roll or to apply holds the member's part; only the
		// master may pass it over, which drops the damage (RN-02: he has the last
		// word).
		if dropped, err = c.pendingOf(ctx, current.ID); err != nil {
			return nil, err
		}
		if len(dropped) > 0 {
			if !v.master || !req.Msg.GetDiscardPendingDamage() {
				return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_PENDING_DAMAGE, "a damage is still to roll or to apply")
			}
			for i, p := range dropped {
				if dropped[i], err = c.q.SetPendingDamageStatus(ctx, playdb.SetPendingDamageStatusParams{ID: p.ID, Status: pendingDiscarded, ResolvedAt: &c.now}); err != nil {
					return nil, fmt.Errorf("discard the pending damage: %w", err)
				}
				// One event for each, written before the turn's own, so the log shows
				// the attack as dropped and an undo stays closed once the turn passes.
				tgt, _ := findCombatant(cs, p.TargetID, combatViewer{master: true})
				if err := insertEvent(ctx, c, eventDamageDiscarded, &m.UserID, nil, actionEvent{
					Round: c.enc.Round, Secret: secretOf(current, tgt), Actor: current.ID, Target: p.TargetID, Pending: p.ID, Key: p.AttackKey,
					Amount: num(p.Amount), PrevStatus: p.Status,
				}); err != nil {
					return nil, err
				}
			}
		}
		c.characterID = &current.CharacterID
		if err := c.q.EndCombatantTurnPart(ctx, current.ID); err != nil {
			return nil, fmt.Errorf("end the part: %w", err)
		}
		// Who still acts: the turn passes only when the last member ends.
		if acting := othersActing(cs, current.ID); len(acting) > 0 {
			// A part of a group of NPCs alone is the master's, like the group (RN-20),
			// and so is a hidden member's: no line for the players.
			holdsPlayer := slices.ContainsFunc(cs, func(o playdb.Combatant) bool { return o.TurnState != turnIdle && inParty(o) })
			secret = current.Hidden || !holdsPlayer
			c.kind = eventTurnPartEnded
			if err := setCurrent(ctx, c, acting[0], c.enc.Round); err != nil {
				return nil, err
			}
			return actionEvent{Round: c.enc.Round, Secret: secret, Actor: current.ID}, nil
		}
		next, newRound, ok := nextTurnGroup(cs, current.ID, "")
		if !ok {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_NOT_ACTIVE, "no combatant can take a turn")
		}
		round := c.enc.Round
		if newRound {
			round++
		}
		// The next group's turn starts: every member's movement, action, bonus
		// action, dash and reaction come back.
		passed = true
		if err := startTurn(ctx, c, next, round); err != nil {
			return nil, err
		}
		return map[string]any{"from": current.ID, "to": next[0], "round": round, "discarded": len(dropped)}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "end a turn", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishTurnChanged(ctx, m.CampaignID, d)
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		if len(dropped) > 0 { // the attacks they belong to show them dropped
			s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !touchesHidden(d.cs, dropped))
		}
		if !passed { // a part ended: its line, which a hidden member keeps from the players
			s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, !secret)
		}
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.EndTurnResponse{Encounter: out}), nil
}

// SetCombatantHidden implements playv1connect.CombatServiceHandler.
func (s *Service) SetCombatantHidden(
	ctx context.Context,
	req *connect.Request[playv1.SetCombatantHiddenRequest],
) (*connect.Response[playv1.SetCombatantHiddenResponse], error) {
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
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}
	hidden := req.Msg.GetHidden()

	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventCombatantHiddenSet, encounterID: encID}, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		target, err := findCombatant(cs, combID, viewerOf(m))
		if err != nil {
			return nil, err
		}
		if target.Kind != kindNPC {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a player's combatant is never hidden"))
		}
		if err := c.q.SetCombatantHidden(ctx, playdb.SetCombatantHiddenParams{ID: target.ID, Hidden: hidden}); err != nil {
			return nil, fmt.Errorf("hide the combatant: %w", err)
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		c.characterID = &target.CharacterID
		return map[string]any{"combatant_id": target.ID, "hidden": hidden, "round": c.enc.Round}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "hide or show a combatant", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, false) // the master's line only
		// If it is in the turn, the players' copy of the turn changes.
		if i := slices.IndexFunc(d.cs, func(o playdb.Combatant) bool { return o.ID == combID }); i >= 0 && inTurn(d.enc, d.cs[i]) {
			s.publishTurnChanged(ctx, m.CampaignID, d)
		}
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SetCombatantHiddenResponse{Encounter: out}), nil
}

// AddCombatants implements playv1connect.CombatServiceHandler.
func (s *Service) AddCombatants(
	ctx context.Context,
	req *connect.Request[playv1.AddCombatantsRequest],
) (*connect.Response[playv1.AddCombatantsResponse], error) {
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
	parts, err := s.participants(ctx, m.CampaignID, req.Msg.GetParticipants(), false, false)
	if err != nil {
		return nil, err
	}

	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventCombatantsAdded, encounterID: encID}, func(c *combatTx) (any, error) {
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		count := len(cs)
		for _, p := range parts {
			count += p.count
			if !p.char.Player {
				continue
			}
			// A player's character cannot be rolled for later in the order.
			if c.enc.Status != statusSetup || slices.ContainsFunc(cs, func(o playdb.Combatant) bool { return o.CharacterID == p.char.ID }) {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("a player's character joins only before the combat begins, and once"))
			}
		}
		if count > maxCombatants {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("a combat has at most %d combatants", maxCombatants))
		}
		_, added, err := s.addParticipants(ctx, c, link.Grid{Columns: c.enc.GridColumns, Rows: c.enc.GridRows}, cs, parts)
		if err != nil {
			return nil, err
		}
		if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
			return nil, fmt.Errorf("touch the encounter: %w", err)
		}
		return map[string]any{"added": len(added)}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "add combatants", err)
	}
	out, err := s.finish(ctx, m, res, s.changed(m.CampaignID))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.AddCombatantsResponse{Encounter: out}), nil
}

// RemoveCombatant implements playv1connect.CombatServiceHandler.
func (s *Service) RemoveCombatant(
	ctx context.Context,
	req *connect.Request[playv1.RemoveCombatantRequest],
) (*connect.Response[playv1.RemoveCombatantResponse], error) {
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
	combID, err := parseCombatID(req.Msg.GetCombatantId(), "combatant")
	if err != nil {
		return nil, err
	}

	var turnPassed bool
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventCombatantRemoved, encounterID: encID}, func(c *combatTx) (any, error) {
		turnPassed = false
		if err := notEnded(c.enc); err != nil {
			return nil, err
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		target, err := findCombatant(cs, combID, viewerOf(m))
		if err != nil {
			return nil, err
		}
		if target.Kind == kindPlayer && c.enc.Status == statusActive {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_PLAYER_IN_COMBAT, "a player's combatant cannot leave a combat that is running")
		}
		// A damage still to roll or to apply that names the combatant (as the attacker or
		// as the target) goes with it, and the log would keep an attack whose damage never
		// lands: the master rolls, applies or discards it first.
		open, err := c.q.ListOpenPendingDamages(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the pending damage: %w", err)
		}
		if slices.ContainsFunc(open, func(p playdb.PendingDamage) bool { return p.TargetID == target.ID || deref(p.AttackerID) == target.ID }) {
			return nil, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_PENDING_DAMAGE, "a damage is still to roll or to apply")
		}
		// A player's character that leaves a combat in SETUP takes its creatures out
		// with it.
		if target.Kind == kindPlayer {
			owned := slices.DeleteFunc(slices.Clone(cs), func(o playdb.Combatant) bool { return !isCreature(o) || o.CharacterID != target.CharacterID })
			if len(owned) > 0 {
				if err := s.writeBackCreatures(ctx, c, owned); err != nil {
					return nil, err
				}
				if cs, _, err = s.dropCombatants(ctx, c, cs, owned, false); err != nil {
					return nil, err
				}
			}
		}
		// A creature that leaves the fight keeps the hit points it has.
		if isCreature(target) {
			if err := s.writeBackCreatures(ctx, c, []playdb.Combatant{target}); err != nil {
				return nil, err
			}
		}
		// It leaves the turn first; the turn passes when nobody who acts is left.
		if turnPassed, err = leaveTurn(ctx, c, cs, target); err != nil {
			return nil, err
		}
		turnPassed = turnPassed || inTurn(c.enc, target)
		if err := c.q.DeleteCombatant(ctx, target.ID); err != nil {
			return nil, fmt.Errorf("delete the combatant: %w", err)
		}
		rest := slices.DeleteFunc(slices.Clone(cs), func(o playdb.Combatant) bool { return o.ID == target.ID })
		if _, err := saveOrder(ctx, c.q, cs, rest); err != nil {
			return nil, err
		}
		c.characterID = &target.CharacterID
		return map[string]any{"combatant_id": target.ID}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "remove a combatant", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		if turnPassed {
			s.publishTurnChanged(ctx, m.CampaignID, d)
		}
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.RemoveCombatantResponse{Encounter: out}), nil
}

// EndEncounter implements playv1connect.CombatServiceHandler.
func (s *Service) EndEncounter(
	ctx context.Context,
	req *connect.Request[playv1.EndEncounterRequest],
) (*connect.Response[playv1.EndEncounterResponse], error) {
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

	var ended bool
	res, err := s.write(ctx, combatWrite{m: m, key: key, hash: idem.Hash(req.Msg), kind: eventEncounterEnded, encounterID: encID}, func(c *combatTx) (any, error) {
		ended = false
		if c.enc.Status == statusEnded {
			return nil, nil // ended before: nothing changes, and no event
		}
		cs, err := c.q.ListCombatants(ctx, c.enc.ID)
		if err != nil {
			return nil, fmt.Errorf("list the combatants: %w", err)
		}
		if err := s.endEncounter(ctx, c, cs); err != nil {
			return nil, err
		}
		ended = true
		return map[string]any{"round": c.enc.Round}, nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "end an encounter", err)
	}
	out, err := s.finish(ctx, m, res, func(ctx context.Context, d *encounterData) {
		if !ended {
			return
		}
		s.publishCreaturesOfCombat(m.CampaignID, d.cs)
		s.publishEncounterChanged(ctx, m.CampaignID, d.enc)
		s.publishLogChanged(ctx, m.CampaignID, d.enc.ID, true)
		if d.enc.MapID != nil {
			// The player characters' tokens moved with the combat's end.
			s.Publish(m.CampaignID, true, &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_MapChanged_{
				MapChanged: &playv1.WatchGameSessionResponse_MapChanged{MapId: *d.enc.MapID},
			}})
		}
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.EndEncounterResponse{Encounter: out}), nil
}

// endEncounter ends the combat c.enc inside the transaction: its status, and
// the player characters' tokens, and the creatures' that have one, which move to the
// squares where they ended (the map shows them where they stand). cs are its combatants.
func (s *Service) endEncounter(ctx context.Context, c *combatTx, cs []playdb.Combatant) error {
	if err := s.keepTrapDamage(ctx, c, cs); err != nil {
		return err
	}
	ended := c.now
	enc, err := c.q.SetEncounterState(ctx, playdb.SetEncounterStateParams{
		ID: c.enc.ID, Status: statusEnded, Round: c.enc.Round, StartedAt: c.enc.StartedAt, EndedAt: &ended,
	})
	if err != nil {
		return fmt.Errorf("end the encounter: %w", err)
	}
	c.enc = enc
	// The creatures keep the hit points they had when the fight ended (MR-037).
	if err := s.writeBackCreatures(ctx, c, cs); err != nil {
		return err
	}
	// A familiar's sight begun in the fight ends with it (MR-036).
	if err := s.endCombatSights(ctx, c, cs); err != nil {
		return err
	}
	if enc.MapID == nil {
		return nil // the map was deleted: nothing to write back to
	}
	grid := link.Grid{Columns: enc.GridColumns, Rows: enc.GridRows}
	var positions []link.TokenPosition
	for _, cb := range cs {
		if cb.Kind == kindPlayer && placed(cb) {
			x, y := centerOf(grid, *cb.GridCol, *cb.GridRow)
			positions = append(positions, link.TokenPosition{CharacterID: cb.CharacterID, XBP: x, YBP: y})
		}
		// A creature's token moves too, if the master gave it one (MR-037): the fight
		// never makes a token for it.
		if isCreature(cb) && !cb.Dismissed && placed(cb) {
			x, y := centerOf(grid, *cb.GridCol, *cb.GridRow)
			positions = append(positions, link.TokenPosition{CreatureID: deref(cb.CreatureID), XBP: x, YBP: y})
		}
	}
	if err := s.maps.SetTokenPositions(ctx, c.tx, *enc.MapID, positions, c.now); err != nil {
		return fmt.Errorf("move the tokens: %w", err)
	}
	return nil
}

// endOpenEncounter ends the session's combat, if it has one, when the master
// ends the session (EndGameSession), inside its transaction, with a session
// event.
func (s *Service) endOpenEncounter(ctx context.Context, tx pgx.Tx, q *playdb.Queries, session playdb.GameSession, actorUserID string) ([]playdb.Combatant, []*playv1.CharacterVitals, error) {
	enc, err := q.GetOpenEncounter(ctx, session.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("find the open encounter: %w", err)
	}
	cs, err := q.ListCombatants(ctx, enc.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("list the combatants: %w", err)
	}
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, enc: enc, now: s.now(), svc: s})
	if err != nil {
		return nil, nil, err
	}
	if err := s.endEncounter(ctx, c, cs); err != nil {
		return nil, nil, err
	}
	return cs, c.told, insertEvent(ctx, c, eventEncounterEnded, &actorUserID, nil, map[string]any{"round": c.enc.Round, "reason": "session_ended"})
}
