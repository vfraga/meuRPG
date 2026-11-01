package play

import (
	"context"
	"fmt"
	"slices"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// reactionEvent is what a reaction did, as the session event keeps it: IDs and
// numbers only. The combat log builds the line of each reader from it (the
// master's with the numbers of NPCs, the players' without them), and the undo of
// a Shield reads the legacy fields of the event around it.
type reactionEvent struct {
	// Hold marks the event that stands for a held action: never a log line, and
	// nothing an undo can take back. Group is the trigger.
	Hold  bool   `json:"hold,omitempty"`
	Group string `json:"group,omitempty"`
	// Kind is the reaction answered (reaction.Kind), Window the window and Used
	// false for "Deixar passar".
	Kind     string `json:"kind,omitempty"`
	Window   string `json:"window,omitempty"`
	Used     bool   `json:"used,omitempty"`
	ByMaster bool   `json:"by_master,omitempty"`
	// Slot is the slot spent, Racial the Infernal Legacy.
	Slot   *slotRef `json:"slot,omitempty"`
	Racial bool     `json:"racial,omitempty"`
	// Spell and Level are what a Counterspell was about, Countered whether it
	// failed the spell, and the check when it needed one.
	Spell     string `json:"spell,omitempty"`
	Level     int32  `json:"level,omitempty"`
	Countered bool   `json:"countered,omitempty"`
	Check     int32  `json:"check,omitempty"`
	CheckDC   int32  `json:"check_dc,omitempty"`
	// Stroke is the use of Stroke of Luck on the Counterspell check (held_stroke.go): Check holds the 20 counted, and the undo gives the use back.
	Stroke *strokeUse `json:"stroke,omitempty"`
	// Cutting Words, Deflect Missiles and Uncanny Dodge: what the roll or the
	// damage was and became, and the die that came off.
	Roll     string `json:"roll,omitempty"`
	Before   int32  `json:"before,omitempty"`
	After    int32  `json:"after,omitempty"`
	Die      int32  `json:"die,omitempty"`
	DieSides int32  `json:"die_sides,omitempty"`
	Ineffect bool   `json:"ineffect,omitempty"`
	AC       int32  `json:"ac,omitempty"`
	Outcome  string `json:"outcome,omitempty"`
	Caught   bool   `json:"caught,omitempty"`
	// ManeuverName and Resource: the table maneuver a maneuver_reduce used and the resource it spent.
	ManeuverName string `json:"maneuver_name,omitempty"`
	Resource     string `json:"resource,omitempty"`
	Stopped      bool   `json:"stopped,omitempty"`
	Magic        bool   `json:"magic,omitempty"`
	// Hellish Rebuke: the aggressor's save and the fire.
	SaveD20   int32 `json:"save_d20,omitempty"`
	SaveBonus int32 `json:"save_bonus,omitempty"`
	SaveDC    int32 `json:"save_dc,omitempty"`
	Saved     bool  `json:"saved,omitempty"`
	Fire      int32 `json:"fire,omitempty"`
	// HeldDropped marks the event of a held action that could not happen after its answers (its
	// kind in Roll, the refusal in Why): a line for the master only.
	Why string `json:"why,omitempty"`
	// Concentration: the spell, the DC and whether it was kept.
	Kept bool `json:"kept,omitempty"`
	// Feather Fall: the creatures saved.
	Saved2 []string `json:"saved_ids,omitempty"`
	// A reaction that was not answered but closed ("A Contramágica do Mago 2 fechou")
	// is no event: only answers are written.
}

// reactionHeldDropped is the kind of the event that says a held action was dropped.
const reactionHeldDropped = "held_dropped"

// reactionNames are the Portuguese names of the reactions, as names_pt.json has them
// (TestReactionNamesAreTheOfficialOnes keeps them equal).
var reactionNames = map[reaction.Kind]string{
	reaction.Shield:             "Escudo Arcano",
	reaction.UncannyDodgeKind:   "Esquiva Sobrenatural",
	reaction.HellishRebukeKind:  "Repreensão Infernal",
	reaction.CounterspellKind:   "Contramágica",
	reaction.CuttingWords:       "Palavras de Interrupção",
	reaction.DeflectKind:        "Defletir Projéteis",
	reaction.ManeuverReduceKind: "Manobra",
	reaction.ProtectionKind:     "Estilo de Luta: Proteção",
	reaction.FeatherFall:        "Queda Suave",
	reaction.Concentration:      "Teste de concentração",
	reaction.MasterCheck:        "Verificação do mestre",
}

// reactionNameKeys are the content keys of the reaction names.
var reactionNameKeys = map[reaction.Kind]string{
	reaction.Shield:            "spell:shield",
	reaction.UncannyDodgeKind:  "feature:uncanny-dodge",
	reaction.HellishRebukeKind: "spell:hellish-rebuke",
	reaction.CounterspellKind:  "spell:counterspell",
	reaction.CuttingWords:      "feature:cutting-words",
	reaction.DeflectKind:       "feature:deflect-missiles",
	reaction.FeatherFall:       "spell:feather-fall",
	reaction.ProtectionKind:    "feature:fighter-fighting-style-protection",
}

func windowOpenedMessage(encounterID, windowID string) *playv1.WatchGameSessionResponse {
	return &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_ReactionWindowOpened_{
		ReactionWindowOpened: &playv1.WatchGameSessionResponse_ReactionWindowOpened{EncounterId: encounterID, WindowId: windowID},
	}}
}

func windowClosedMessage(encounterID, windowID string, byItself bool, text string) *playv1.WatchGameSessionResponse {
	return &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_ReactionWindowClosed_{
		ReactionWindowClosed: &playv1.WatchGameSessionResponse_ReactionWindowClosed{
			EncounterId: encounterID, WindowId: windowID, ClosedByItself: byItself, TextPt: text,
		},
	}}
}

// publishReactions tells, after the commit, the windows a change opened and closed:
// to the reactor's player and to the master, nobody else (RN-10); the others read the
// change of the wait in the combat. It also tells the vitals a replayed action
// changed.
func (s *Service) publishReactions(ctx context.Context, campaignID string, res combatResult) {
	if res.repeated || (len(res.notes) == 0 && len(res.tellIDs) == 0) {
		return
	}
	cs, err := s.queries.ListCombatants(ctx, res.encounterID)
	if err != nil {
		s.logger.WarnContext(ctx, "play: cannot read the combatants to tell the reactions", "error", err)
		return
	}
	byID := combatantsByID(cs)
	for _, n := range res.notes {
		s.publishNote(campaignID, res.encounterID, n, byID)
	}
	for _, id := range res.tellIDs {
		if x, ok := byID[id]; ok && x.Kind == kindPlayer {
			if v, err := s.vitals.GetVitals(ctx, campaignID, x.CharacterID); err == nil {
				s.publishVitals(campaignID, v)
			}
		}
	}
}

// publishNote tells one window that opened or closed to its reactor's player and to the
// master.
func (s *Service) publishNote(campaignID, encounterID string, n reactionNote, byID map[string]playdb.Combatant) {
	w := n.window
	reactor, hasReactor := byID[conv.OrZero(w.ReactorID)]
	audience := []live.Audience{{Master: true}}
	if hasReactor && reactor.UserID != nil {
		audience = append(audience, live.Audience{UserID: *reactor.UserID})
	}
	for i, a := range audience {
		if n.opened {
			s.hub.Publish(campaignID, live.Event{Audience: a, Message: windowOpenedMessage(encounterID, w.ID)})
			continue
		}
		text := ""
		if n.byItself {
			text = closedText(n, reactor, hasReactor, i == 0)
		}
		s.hub.Publish(campaignID, live.Event{Audience: a, Message: windowClosedMessage(encounterID, w.ID, n.byItself, text)})
	}
}

// closedText is the sentence a window that closed by itself leaves: the reactor reads
// only the reason about itself (never that another reactor acted), the master the
// reason in the fight's words (PM-04c, 11).
func closedText(n reactionNote, reactor playdb.Combatant, hasReactor, forMaster bool) string {
	kind := reaction.Kind(n.window.Kind)
	name := reactionNames[kind]
	label := ""
	if hasReactor {
		label = reactor.Label
	}
	if forMaster {
		switch n.reason {
		case reaction.ReasonReactionSpent:
			return fmt.Sprintf("%s de %s fechou: a reação já foi usada.", name, label)
		case reaction.ReasonReactorIncapacitated:
			return fmt.Sprintf("%s de %s fechou: não pode reagir.", name, label)
		}
		switch kind {
		case reaction.CounterspellKind:
			return fmt.Sprintf("A Contramágica de %s fechou: a magia já foi anulada.", label)
		case reaction.Concentration:
			return fmt.Sprintf("O teste de concentração de %s fechou: a concentração já acabou.", label)
		}
		return fmt.Sprintf("%s de %s fechou: o gatilho não vale mais.", name, label)
	}
	switch n.reason {
	case reaction.ReasonReactionSpent:
		if used, ok := reactionNames[n.spentOn]; ok && n.spentOn != "" {
			return fmt.Sprintf("%s fechou. Você já usou a sua reação (%s). Ela volta no começo do seu próximo turno.", name, used)
		}
		return fmt.Sprintf("%s fechou. Você já usou a sua reação. Ela volta no começo do seu próximo turno.", name)
	case reaction.ReasonReactorIncapacitated:
		if hasReactor && (reactor.Defeated || slices.Contains(reactor.Conditions, "condition:unconscious")) {
			return fmt.Sprintf("%s fechou. Você está inconsciente e não pode reagir.", name)
		}
		return fmt.Sprintf("%s fechou. Você não pode reagir agora.", name)
	}
	return name + " fechou."
}
