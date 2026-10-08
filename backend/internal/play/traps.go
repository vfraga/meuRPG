package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	maplink "github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Traps in play, the session's side (MR-035, Etapa 9, D5, slice 9.8; RN-10,
// RN-02): the player's "Procurar armadilhas", the master's firing by hand and
// the damage a trap does to a player's character while no combat runs. The
// moves of a combat and the combat's own firing are combat_traps.go's; the dice
// are traps_effect.go's. The maps module owns the trap itself and tells this one,
// through TrapBook, everything that is a map's: where the area is, what a
// character sees of it, who knows it.
//
// What a player never receives: a trap their characters do not know, in any
// response or stream event. A search that finds nothing reads the same as one that
// failed; a trap that fired is public.

// TrapBook is what the play module asks of the maps module for the traps (MR-035,
// D5). *maps.Service implements it; cmd/api connects it with SetTraps, after both
// services exist (play and maps need each other). Nil means no trap does anything
// in play. The methods take no caller: they run after this package's own
// authorization, and the maps module never sends a trap's data to a player.
type TrapBook interface {
	// Traps returns the map's traps with the master's data (DCs, effect), or a
	// `not_found` Connect error when it is not a map of the campaign.
	Traps(ctx context.Context, tx pgx.Tx, campaignID, mapID string) ([]maplink.Trap, error)
	// KnownTraps returns the armed traps the character knows, with their squares
	// and names and without their data.
	KnownTraps(ctx context.Context, campaignID, mapID, characterID string) ([]maplink.Trap, error)
	// Notice is the passive notice of observers that ended a move (see
	// maps.Service.Notice): it reveals the traps they notice to their characters
	// and tells only their players.
	Notice(ctx context.Context, campaignID, mapID string, who []maplink.Observer) error
	// SearchTraps is "Procurar armadilhas" with the roll's total: it reveals, inside
	// tx, the traps the observer finds to its character.
	SearchTraps(ctx context.Context, tx pgx.Tx, campaignID, mapID string, who maplink.Observer, skill string, totals []int, at time.Time) (maplink.SearchResult, error)
	// Told tells the players, after the commit, that the map changed for them.
	Told(ctx context.Context, campaignID, mapID string, users []string)
	// TriggerTrap marks the trap triggered inside tx and returns it as it was;
	// maplink.ErrTrapNotArmed when it is not armed. RestoreTrap is its undo.
	TriggerTrap(ctx context.Context, tx pgx.Tx, campaignID, mapID, pointID string, at time.Time) (maplink.Trap, error)
	RestoreTrap(ctx context.Context, tx pgx.Tx, campaignID, mapID, pointID, state string, triggeredAt *time.Time, at time.Time) error
	// TrapChanged tells the watching members, after the commit, that a trap's state
	// changed (it fired, or an undo armed it again).
	TrapChanged(ctx context.Context, campaignID, mapID, pointID string)
	// TrapNames returns the names of the traps among the points, by point ID: a trap
	// that fired is public, so its name may go into the log.
	TrapNames(ctx context.Context, campaignID string, pointIDs []string) (map[string]string, error)
}

// SetTraps connects the maps module's traps. play and maps need each other, so
// cmd/api calls it once maps exists, before the server starts.
func (s *Service) SetTraps(b TrapBook) { s.traps = b }

// The skills a search rolls, and the keys of their checks.
const (
	searchPerception    = "perception"
	searchInvestigation = "investigation"
)

var searchCheckKey = map[string]string{searchPerception: "skill:perception", searchInvestigation: "skill:investigation"}

// errNoTraps is the error of a server with the traps off (no maps module wired).
func errNoTraps() error {
	return connect.NewError(connect.CodeUnavailable, errors.New("traps are not available on this server"))
}

func errTrapNotFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("trap not found"))
}

// errTrapNotArmed is the failed_precondition of firing a trap that is not armed.
func errTrapNotArmed() error {
	return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TRAP_NOT_ARMED, "the trap is not armed")
}

// runningEncounterOn is the session's combat on the map that is not ended.
func (s *Service) runningEncounterOn(ctx context.Context, sessionID, mapID string) (playdb.Encounter, bool, error) {
	enc, err := s.queries.GetLatestEncounter(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.Encounter{}, false, nil
	}
	if err != nil {
		return playdb.Encounter{}, false, fmt.Errorf("find the session's combat: %w", err)
	}
	return enc, enc.Status != statusEnded && enc.MapID != nil && *enc.MapID == mapID, nil
}

// errCombatBegan is the refusal of a firing outside a combat when a combat began on the
// trap's map after the firing was read: the trap fires from the combat.
func errCombatBegan() error {
	return connect.NewError(connect.CodeFailedPrecondition, errors.New("a combat began on this map: fire the trap from the combat"))
}

// ---- Procurar armadilhas ----

// SearchForTraps implements playv1connect.PlayServiceHandler.
func (s *Service) SearchForTraps(
	ctx context.Context,
	req *connect.Request[playv1.SearchForTrapsRequest],
) (*connect.Response[playv1.SearchForTrapsResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RolePlayer)
	if err != nil {
		return nil, err
	}
	key, err := parseKey(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	hash := idem.Hash(req.Msg)
	skill := ""
	switch req.Msg.GetSkill() {
	case playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_PERCEPTION:
		skill = searchPerception
	case playv1.TrapSearchSkill_TRAP_SEARCH_SKILL_INVESTIGATION:
		skill = searchInvestigation
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("skill must be PERCEPTION or INVESTIGATION"))
	}
	var in rollInput
	switch roll := req.Msg.GetRoll().(type) {
	case *playv1.SearchForTrapsRequest_RollInApp:
		if !roll.RollInApp {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("roll_in_app must be true"))
		}
		in.inApp = true
	case *playv1.SearchForTrapsRequest_D20Face:
		in.typed = int(roll.D20Face)
		if in.typed < 1 || in.typed > 20 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face must be 1 to 20"))
		}
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("set roll_in_app or d20_face"))
	}
	var typed2 int
	if f := req.Msg.D20Face_2; f != nil {
		if !in.inApp && skill == searchPerception {
			typed2 = int(*f)
			if typed2 < 1 || typed2 > 20 {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("d20_face_2 must be 1 to 20"))
			}
		}
	}
	if s.traps == nil {
		return nil, errNoTraps()
	}
	who, has, err := s.myCharacter(ctx, nil, m)
	if err != nil {
		return nil, s.dbError(ctx, "find the caller's character", err)
	}
	if !has {
		return nil, errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_CHARACTER, "you have no living character to search with")
	}

	var ev actionEvent
	var repeated bool
	var told []string
	var place searchPlace
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		repeated, told, place = false, nil, searchPlace{}
		session, err := q.GetOpenGameSessionForUpdate(ctx, m.CampaignID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoOpenSession()
		}
		if err != nil {
			return fmt.Errorf("lock the open session: %w", err)
		}
		// A retry of a search already made returns that search, and changes nothing.
		done, err := q.GetSessionEventByIdempotencyKey(ctx, playdb.GetSessionEventByIdempotencyKeyParams{GameSessionID: session.ID, IdempotencyKey: &key})
		switch {
		case err == nil:
			if done.Kind != eventTrapSearched || done.ActorUserID == nil || *done.ActorUserID != m.UserID || hashDiffers(done.IdempotencyHash, hash) {
				return connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key was already used for another change"))
			}
			repeated = true
			return json.Unmarshal(done.Payload, &ev)
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("find the event of this idempotency key: %w", err)
		}
		if theatre, err := theatreRunning(ctx, q, session.ID); err != nil {
			return err
		} else if theatre {
			return errNeedsAMap() // a combat without a map has no traps to search for (RN-25)
		}
		force, err := s.dice.ForcedDice(ctx, tx, m.CampaignID, m.UserID)
		if err != nil {
			return err
		}
		if force.refuses(in.inApp) {
			return errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_WRONG_DICE_MODE, "this is not how the campaign has you roll your dice")
		}
		options, err := s.roster.SceneOptions(ctx, tx, m.CampaignID, who.ID, []string{searchCheckKey[skill]})
		if err != nil {
			return err
		}
		if len(options) != 1 || !options[0].Known {
			return errScene(playv1.SceneBlockedReason_SCENE_BLOCKED_REASON_NO_CHARACTER, "your character has no numbers for this check")
		}

		c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now(), characterID: &who.ID, kind: eventTrapSearched, actorUserID: m.UserID, hash: hash})
		if err != nil {
			return err
		}
		if place, err = s.searcherPlace(ctx, c, who); err != nil {
			return err
		}
		face, roll, err := s.d20(in, options[0].Bonus)
		if err != nil {
			return err
		}
		totals := []int{roll.Total}
		// A Perception search is rolled twice in the app (the lower counts where the
		// light is dim to the searcher); with a real die the second one is typed.
		var face2 int
		switch {
		case skill != searchPerception:
		case in.inApp:
			var roll2 dice.Result
			if face2, roll2, err = s.d20(in, options[0].Bonus); err != nil {
				return err
			}
			totals = append(totals, roll2.Total)
		case typed2 > 0:
			face2 = typed2
			totals = append(totals, typed2+options[0].Bonus)
		}
		found, err := s.traps.SearchTraps(ctx, tx, m.CampaignID, place.mapID, maplink.Observer{CharacterID: who.ID, At: place.at}, skill, totals, c.now)
		if errors.Is(err, maplink.ErrSearchNeedsTwoDice) {
			return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_SEARCH_NEEDS_TWO_DICE, "a Perception search in dim light has disadvantage: roll a second die")
		}
		if err != nil {
			return err
		}
		told = found.Users
		ev = actionEvent{
			Round: c.enc.Round, Actor: place.combatantID, Key: skill, D20: clampInt32(face), Modifier: clampInt32(options[0].Bonus), Total: clampInt32(roll.Total),
			Physical: roll.Physical, Found: found.Found, OnTurn: place.spent, D20B: clampInt32(face2),
		}
		if place.spent {
			if c.enc, err = c.q.TouchEncounter(ctx, c.enc.ID); err != nil {
				return fmt.Errorf("touch the encounter: %w", err)
			}
			place.enc = c.enc
		}
		// Only the master and the searcher hear of the roll (RN-20); the history keeps
		// the ids and the numbers, never the DC.
		return insertEvent(ctx, c, eventTrapSearched, &m.UserID, &key, ev)
	})
	if err != nil {
		return nil, s.dbError(ctx, "search for traps", err)
	}
	if !repeated {
		if len(ev.Found) > 0 {
			logging.Event(ctx, s.logger, "trap.noticed", slog.String("map_id", place.mapID), slog.Int("found", len(ev.Found)), slog.Bool("physical_dice", ev.Physical))
		}
		if len(told) > 0 {
			s.traps.Told(ctx, m.CampaignID, place.mapID, told)
		}
		if place.spent {
			s.publishEncounterChanged(ctx, m.CampaignID, place.enc)
			s.publishLogChanged(ctx, m.CampaignID, place.enc.ID, false) // the master's log has the search
		}
		s.Publish(m.CampaignID, false, mapChangedHint(place.mapID)) // the master's activity read has a new line: a hint with no content
	}
	out := &playv1.SearchForTrapsResponse{
		Roll:          diceRoll(1, 20, []int32{ev.D20}, ev.Modifier, ev.Total, ev.Physical),
		FoundPointIds: ev.Found,
		SpentAction:   ev.OnTurn,
	}
	if ev.D20B != 0 {
		out.SecondRoll = diceRoll(1, 20, []int32{ev.D20B}, ev.Modifier, ev.D20B+ev.Modifier, ev.Physical)
	}
	return connect.NewResponse(out), nil
}

// searchPlace is where a character searches from, and what the search costs.
type searchPlace struct {
	mapID string
	at    grid.Square
	// enc is the combat the character is in and combatantID its combatant; spent
	// says the search took the action of its turn.
	enc         playdb.Encounter
	combatantID string
	spent       bool
}

// searcherPlace finds where the character searches from, inside the change's
// transaction. While a combat is running and the character is one of its
// combatants it is the SRD's Search action: the character's own turn, one action,
// from its combatant's square on the combat's map; the action is spent here. Any
// other time it is the session's current map and the character's token on it.
func (s *Service) searcherPlace(ctx context.Context, c *combatTx, who link.Character) (searchPlace, error) {
	notOnMap := func() error {
		return errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TRAP_NOT_ON_MAP, "your character is not on a map with a grid")
	}
	latest, err := c.q.GetLatestEncounter(ctx, c.session.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return searchPlace{}, fmt.Errorf("find the session's combat: %w", err)
	}
	if err == nil && latest.Status != statusEnded {
		cs, err := c.q.ListCombatants(ctx, latest.ID)
		if err != nil {
			return searchPlace{}, fmt.Errorf("list the combatants: %w", err)
		}
		if i := slices.IndexFunc(cs, func(o playdb.Combatant) bool { return o.Kind == kindPlayer && o.CharacterID == who.ID }); i >= 0 {
			me := cs[i]
			if latest.Status != statusActive {
				return searchPlace{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_TRAP_SEARCH_NOT_NOW, "the combat has not begun: search when it is your turn")
			}
			c.enc = latest
			if err := s.mustActNow(ctx, c, me); err != nil {
				return searchPlace{}, err
			}
			if me.ActionUsed {
				return searchPlace{}, errEncounter(playv1.EncounterBlockedReason_ENCOUNTER_BLOCKED_REASON_ACTION_USED, "the action of this turn is used")
			}
			if latest.MapID == nil || !placed(me) {
				return searchPlace{}, notOnMap()
			}
			run, err := breakRun(ctx, c, me)
			if err != nil {
				return searchPlace{}, err
			}
			_ = run // a search is no undoable action: the running start is simply lost
			if err := c.q.SetCombatantEconomy(ctx, playdb.SetCombatantEconomyParams{
				ID: me.ID, ActionUsed: true, BonusActionUsed: me.BonusActionUsed, ReactionUsed: me.ReactionUsed, Dashed: me.Dashed,
			}); err != nil {
				return searchPlace{}, fmt.Errorf("spend the action: %w", err)
			}
			return searchPlace{mapID: *latest.MapID, at: squareOfCombatant(me), enc: latest, combatantID: me.ID, spent: true}, nil
		}
	}
	if c.session.CurrentMapID == nil {
		return searchPlace{}, notOnMap()
	}
	mapID := *c.session.CurrentMapID
	g, err := s.maps.MapGrid(ctx, c.tx, c.session.CampaignID, mapID)
	if err != nil {
		return searchPlace{}, err
	}
	tokens, err := s.maps.MapTokens(ctx, c.tx, mapID)
	if err != nil {
		return searchPlace{}, err
	}
	i := slices.IndexFunc(tokens, func(t link.TokenPosition) bool { return t.CharacterID == who.ID })
	if !g.OK() || i < 0 {
		return searchPlace{}, notOnMap()
	}
	return searchPlace{mapID: mapID, at: grid.Grid{Columns: int(g.Columns), Rows: int(g.Rows)}.SquareOf(int(tokens[i].XBP), int(tokens[i].YBP))}, nil
}

// mapChangedHint is the stream event that makes a watcher read the map again.
func mapChangedHint(mapID string) *playv1.WatchGameSessionResponse {
	return &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_MapChanged_{
		MapChanged: &playv1.WatchGameSessionResponse_MapChanged{MapId: mapID},
	}}
}
