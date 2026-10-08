package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The session summary, "Resumo da sessão" (MR-032, Etapa 8; question 64 of
// the progress doc). Like the combat highlights, it is worked out on every
// read from the session's events, never stored. It reuses the combat
// highlights' code (tallyHighlights, categoriesOf) for each combat and sums
// the tallies per character, then adds the checks passed outside combat.
//
// The checks count only rolls made in a scene that showed its DC to the
// players at that moment (sceneRollEvent.DCShown), on an action that had one
// (Passed is set). A roll in a scene that hid its DC enters no count, for
// anyone: the scene never told the players whether it was a pass or a fail,
// and the summary must not tell it either (RN-20).
//
// "Mais tesouro encontrado" (MR-041, Etapa 9) is the last category: the PO of
// the treasures marked found while the session was open, per character, split
// among a treasure's finders and rounded down (MapKeeper.TreasureFoundIn). It
// counts in every XP mode, and every member who reads the summary gets it:
// found treasure is public.

// sessionHighlightKinds is the summary's categories: the combat's, then the
// checks passed and the treasure found.
var sessionHighlightKinds = append(slices.Clone(highlightKinds),
	highlightKind{
		kind:  playv1.HighlightKind_HIGHLIGHT_KIND_CHECKS_PASSED,
		value: func(t *characterTally) int32 { return t.checksPassed },
	},
	highlightKind{
		kind:  playv1.HighlightKind_HIGHLIGHT_KIND_TREASURE_FOUND,
		value: func(t *characterTally) int32 { return t.treasurePO },
	},
)

// sessionTally is the numbers of every player's character in a session, in the
// order they first appeared.
type sessionTally struct {
	list []*characterTally
	byID map[string]*characterTally
}

func (st *sessionTally) of(characterID, name string) *characterTally {
	if t, ok := st.byID[characterID]; ok {
		return t
	}
	t := &characterTally{characterID: characterID, name: name}
	if st.byID == nil {
		st.byID = map[string]*characterTally{}
	}
	st.byID[characterID] = t
	st.list = append(st.list, t)
	return t
}

// addCombat sums one combat's tallies in.
func (st *sessionTally) addCombat(tallies []*characterTally) {
	for _, c := range tallies {
		t := st.of(c.characterID, c.name)
		t.damage += c.damage
		t.healing += c.healing
		t.taken += c.taken
		t.finalBlows += c.finalBlows
		t.crits += c.crits
	}
}

// GetSessionSummary implements playv1connect.PlayServiceHandler.
func (s *Service) GetSessionSummary(
	ctx context.Context,
	req *connect.Request[playv1.GetSessionSummaryRequest],
) (*connect.Response[playv1.GetSessionSummaryResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.Msg.GetGameSessionId())
	if err != nil {
		return nil, errSessionNotFound()
	}
	session, err := s.queries.GetGameSessionInCampaign(ctx, playdb.GetGameSessionInCampaignParams{CampaignID: m.CampaignID, ID: id.String()})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errSessionNotFound()
	}
	if err != nil {
		return nil, s.dbError(ctx, "find the session", err)
	}
	if session.EndedAt == nil {
		// Its numbers keep moving until the master ends it, and each combat has
		// its own highlights meanwhile.
		return nil, errBlocked(playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_SESSION_NOT_ENDED,
			"the game session has not ended")
	}

	var st sessionTally
	combats, err := s.queries.ListSessionCombats(ctx, session.ID)
	if err != nil {
		return nil, s.dbError(ctx, "list the session's combats", err)
	}
	for _, enc := range combats {
		events, err := s.queries.ListEncounterCombatEvents(ctx, &enc.ID)
		if err != nil {
			return nil, s.dbError(ctx, "list a combat's events", fmt.Errorf("summary: %w", err))
		}
		cs, err := s.queries.ListCombatants(ctx, enc.ID)
		if err != nil {
			return nil, s.dbError(ctx, "list the combatants", err)
		}
		st.addCombat(tallyHighlights(events, cs))
	}

	scenesOpened, err := s.tallyChecks(ctx, session.ID, &st)
	if err != nil {
		return nil, err
	}

	if err := s.tallyTreasure(ctx, session.ID, &st); err != nil {
		return nil, err
	}

	// The names of the ones who only rolled checks or found treasure, and whose character each
	// one is (for "mine").
	ids := make([]string, 0, len(st.list))
	for _, t := range st.list {
		ids = append(ids, t.characterID)
	}
	chars := map[string]link.Character{}
	if len(ids) > 0 {
		// Whatever their status: a character that died during the session is
		// still in its summary, with its name and its player.
		found, err := s.roster.SessionCharacters(ctx, nil, m.CampaignID, ids)
		if err != nil {
			return nil, s.dbError(ctx, "read the characters' names", err)
		}
		for _, c := range found {
			chars[c.ID] = c
		}
	}
	for _, t := range st.list {
		if t.name == "" {
			t.name = chars[t.characterID].Name
		}
	}

	sum := &playv1.SessionSummary{
		StartedAt:  timestamppb.New(session.StartedAt),
		EndedAt:    timestamppb.New(*session.EndedAt),
		Duration:   durationpb.New(session.EndedAt.Sub(session.StartedAt)),
		Categories: categoriesOf(sessionHighlightKinds, st.list),
	}
	if m.Role == authz.RoleMaster {
		sum.Combats = clamp32(len(combats), 0, 1<<30)
		sum.ScenesOpened = clamp32(scenesOpened, 0, 1<<30)
		for _, t := range st.list {
			sum.ChecksPassed += t.checksPassed
			sum.ChecksTried += t.checksTried
			sum.Players = append(sum.Players, characterSummary(t))
		}
	} else if m.UserID != "" {
		// Every character the caller played in the session, summed: a player
		// that lost a character and went on with another sees both.
		var own []*characterTally
		for _, t := range st.list {
			if chars[t.characterID].PlayerUserID == m.UserID {
				own = append(own, t)
			}
		}
		if len(own) > 0 {
			total := *own[len(own)-1] // named after the latest
			total.damage, total.healing, total.taken, total.finalBlows, total.crits, total.checksPassed, total.checksTried, total.treasurePO = 0, 0, 0, 0, 0, 0, 0, 0
			for _, t := range own {
				total.damage += t.damage
				total.healing += t.healing
				total.taken += t.taken
				total.finalBlows += t.finalBlows
				total.crits += t.crits
				total.checksPassed += t.checksPassed
				total.checksTried += t.checksTried
				total.treasurePO += t.treasurePO
			}
			sum.Mine = characterSummary(&total)
		}
	}
	return connect.NewResponse(&playv1.GetSessionSummaryResponse{Summary: sum}), nil
}

// tallyChecks adds the checks passed and tried outside combat to the tallies,
// and returns how many scenes the master opened. The counting is done by the
// database, so the number of rolls in a session does not matter.
func (s *Service) tallyChecks(ctx context.Context, sessionID string, st *sessionTally) (int, error) {
	opened, err := s.queries.CountSessionScenesOpened(ctx, sessionID)
	if err != nil {
		return 0, s.dbError(ctx, "count the session's scenes", err)
	}
	rows, err := s.queries.TallySessionSceneChecks(ctx, sessionID)
	if err != nil {
		return 0, s.dbError(ctx, "tally the session's checks", err)
	}
	for _, r := range rows {
		if r.CharacterID == nil {
			continue
		}
		t := st.of(*r.CharacterID, "")
		t.checksTried += clampInt32(int(r.Tried))
		t.checksPassed += clampInt32(int(r.Passed))
	}
	return int(opened), nil
}

// tallyTreasure adds the PO each character found in the session to the tallies
// (MR-041). A treasure found by someone who neither fought nor rolled still
// puts them in the summary.
func (s *Service) tallyTreasure(ctx context.Context, sessionID string, st *sessionTally) error {
	found, err := s.maps.TreasureFoundIn(ctx, sessionID)
	if err != nil {
		return s.dbError(ctx, "read the treasures found", err)
	}
	// In the order of the IDs, so the summary does not depend on map order.
	ids := make([]string, 0, len(found))
	for id := range found {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if found[id] > 0 {
			st.of(id, "").treasurePO += found[id]
		}
	}
	return nil
}

// characterSummary is one character's numbers.
func characterSummary(t *characterTally) *playv1.SessionCharacterSummary {
	return &playv1.SessionCharacterSummary{
		Highlights: &playv1.CharacterHighlights{
			CharacterId: t.characterID, Name: t.name,
			DamageDealt: t.damage, HealingDone: t.healing, DamageTaken: t.taken, FinalBlows: t.finalBlows, CriticalHits: t.crits,
		},
		ChecksPassed: t.checksPassed, ChecksTried: t.checksTried, TreasureFoundPo: t.treasurePO,
	}
}
