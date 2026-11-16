package play

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// The session events of the Inspiration the master grants (favor_*.go). Each payload holds ids only, never a name or free text.
const (
	eventFavorGranted = "favor_granted"
	eventFavorSpent   = "favor_spent"
	eventFavorPassed  = "favor_passed"

	eventFavorPassAsked = "favor_pass_asked"
	// eventFavorAskRefused is the master's refusal of a pass (event "favor_pass_refused"): gosec G101 reads a constant named ...PassRefused as a credential.
	eventFavorAskRefused     = "favor_pass_refused"
	eventFavorPassCanceled   = "favor_pass_canceled"
	eventFavorRefundPrompted = "favor_refund_prompted"
	eventFavorRefunded       = "favor_refunded"
	eventFavorRefundKept     = "favor_refund_kept"
)

// favorGrantedEvent is the payload of favor_granted: the character the master gave the Inspiration to.
type favorGrantedEvent struct {
	CharacterID string `json:"character_id"`
}

// favorSpentEvent is the payload of favor_spent: the character that spent the Inspiration. No writer uses it: a spend rides its roll's event (Favor).
type favorSpentEvent struct {
	CharacterID string `json:"character_id"`
}

// favorPassedEvent is the payload of favor_passed: the master accepted the pass, so the character that gave the Inspiration away
// lost it and the one that took it has it now.
type favorPassedEvent struct {
	PassID          string `json:"pass_id"`
	FromCharacterID string `json:"from_character_id"`
	ToCharacterID   string `json:"to_character_id"`
}

// favorPassEvent is the payload of favor_pass_asked, favor_pass_refused and favor_pass_canceled: the pass and its two characters.
type favorPassEvent struct {
	PassID          string `json:"pass_id"`
	FromCharacterID string `json:"from_character_id"`
	ToCharacterID   string `json:"to_character_id"`
}

// favorRefundEvent is the payload of favor_refund_prompted, favor_refunded and favor_refund_kept: the prompt and the character that
// spent the Inspiration.
type favorRefundEvent struct {
	PromptID    string `json:"prompt_id"`
	CharacterID string `json:"character_id"`
}

// favorInCombat is what a favor event carries when it is written while the session's combat is running and its characters are
// combatants of it (decision: the open encounter and its round; additive fields with the names of actionEvent, so the combat's log reads
// them). Actor and Target are the combatants of the giver (or the roller, or the character granted) and of the receiver. Secret says a
// hidden combatant was in it: no player gets its line (RN-10). Fogged and SeenBy are the audience, the owners of the characters named, as
// the fog's stamp reads them, so no other player hears of the event or of its undo.
type favorInCombat struct {
	EncounterID string   `json:"encounter_id"`
	Round       int32    `json:"round"`
	Actor       string   `json:"combatant_id,omitempty"`
	Target      string   `json:"target_id,omitempty"`
	Secret      bool     `json:"secret,omitempty"`
	Fogged      bool     `json:"fogged,omitempty"`
	SeenBy      []string `json:"seen_by,omitempty"`
	// Owner is the owner of the first character named (the giver, the roller, the character granted); it is never stored. A pass that moved
	// nothing (refused, canceled) is the giver's business alone, so only their owner hears of it (stageOfPass).
	Owner string `json:"-"`
}

// The payloads of the favor events written in a combat: the same ids, and the combat's fields.
type (
	favorGrantedCombatEvent struct {
		favorGrantedEvent
		favorInCombat
	}
	favorPassedCombatEvent struct {
		favorPassedEvent
		favorInCombat
	}
	favorPassCombatEvent struct {
		favorPassEvent
		favorInCombat
	}
	favorRefundCombatEvent struct {
		favorRefundEvent
		favorInCombat
	}
)

// favorStage finds where the favor event is written: the session's combat that is running (not in setup), when every character named
// is a player combatant of it. The second result is false outside a combat, and then the event carries nothing of one: no log line, no undo.
func (s *Service) favorStage(ctx context.Context, q *playdb.Queries, sessionID string, characterIDs ...string) (favorInCombat, bool, error) {
	enc, err := q.GetOpenEncounter(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && enc.Status != statusActive) {
		return favorInCombat{}, false, nil
	}
	if err != nil {
		return favorInCombat{}, false, fmt.Errorf("find the open encounter: %w", err)
	}
	cs, err := q.ListCombatants(ctx, enc.ID)
	if err != nil {
		return favorInCombat{}, false, fmt.Errorf("list the combatants: %w", err)
	}
	st := favorInCombat{EncounterID: enc.ID, Round: enc.Round, Fogged: true}
	var ids []string
	for i, id := range characterIDs {
		j := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.Kind == kindPlayer && c.CharacterID == id })
		if j < 0 {
			return favorInCombat{}, false, nil
		}
		ids = append(ids, cs[j].ID)
		st.note(cs[j], i == 0)
	}
	st.Actor = ids[0]
	if len(ids) > 1 {
		st.Target = ids[1]
	}
	slices.Sort(st.SeenBy)
	return st, true, nil
}

// note adds a combatant of the event to the audience: a hidden one makes it secret, and its owner is among those who may hear of it (the
// first named is the Owner).
func (st *favorInCombat) note(c playdb.Combatant, first bool) {
	st.Secret = st.Secret || c.Hidden
	if c.UserID == nil {
		return
	}
	if first {
		st.Owner = *c.UserID
	}
	if !slices.Contains(st.SeenBy, *c.UserID) {
		st.SeenBy = append(st.SeenBy, *c.UserID)
	}
}

// tellFavorLog tells the master, and the owners the line is for, that the combat's log changed. A line with a hidden combatant is the
// master's alone.
func tellFavorLog(h *hints, campaignID string, st favorInCombat) {
	h.tell(campaignID, live.Event{Audience: live.Audience{Master: true}, Message: combatLogChangedMessage(st.EncounterID, false)})
	if st.Secret {
		return
	}
	for _, u := range st.SeenBy {
		h.tell(campaignID, live.Event{Audience: live.Audience{UserID: u}, Message: combatLogChangedMessage(st.EncounterID, false)})
	}
}
