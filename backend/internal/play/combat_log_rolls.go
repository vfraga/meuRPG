package play

import (
	"math"
	"slices"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// What the combat log says of the new rolls: the d20 pair and the mode of an attack, the
// parts of its damage, the steps resistance took it through, the states that began or
// ended, and the master's answers (RN-10, RN-20).
//
//	                         master    attacker's player    target's player    others
//	the d20 pair, the parts  yes       yes                  no                 no
//	the mode change          yes       yes                  no                 no
//	the reason of a change   yes       yes                  no                 no
//	the steps                yes       no                   yes (own character) no
//	an answer of the master  yes       yes                  no                 no
//	a state that began/ended yes       yes                  yes                yes (unhidden)

// logRolls is what the log reads of the combat's own tables: the short texts events
// point at by id, and the requests.
type logRolls struct {
	reasons  map[string]string
	requests map[string]playdb.RollModeRequest
	names    func(string) string
}

func newLogRolls(reasons []playdb.CombatReason, requests []playdb.RollModeRequest, names func(string) string) *logRolls {
	out := &logRolls{reasons: map[string]string{}, requests: map[string]playdb.RollModeRequest{}, names: names}
	for _, r := range reasons {
		out.reasons[r.ID] = r.Reason
	}
	for _, r := range requests {
		out.requests[r.ID] = r
	}
	return out
}

// keyName is the Portuguese name of a content key, or "" without a names reader (a log built apart from the service).
func (r *logRolls) keyName(key string) string {
	if r == nil || r.names == nil {
		return ""
	}
	return r.names(key)
}

// reasonOf is the text behind an event: the reason of a free change, or the one a
// player gave to the master's request.
func (r *logRolls) reasonOf(ev actionEvent) string {
	if r == nil {
		return ""
	}
	if ev.ReasonID != "" {
		return r.reasons[ev.ReasonID]
	}
	if ev.RequestID != "" {
		return r.requests[ev.RequestID].Reason
	}
	return ""
}

// modeChange is the line's mode when it is not the server's suggestion. Its audience is
// the roller's: the master and the attacker's player.
func (e *logEntry) modeChange(ev actionEvent, byMaster bool) *playv1.CombatLogModeChange {
	if ev.SuggestedMode == "" || ev.RollMode == ev.SuggestedMode {
		return nil
	}
	return &playv1.CombatLogModeChange{
		SuggestedMode: modeToProto[modeOfKey(ev.SuggestedMode)], Mode: modeToProto[modeOfKey(ev.RollMode)],
		ByMaster: byMaster, Requested: ev.RequestID != "", Approved: ev.RequestID != "",
	}
}

// answerChange is the master's answer to a player's request.
func (e *logEntry) answerChange() *playv1.CombatLogModeChange {
	return &playv1.CombatLogModeChange{
		SuggestedMode: modeToProto[modeOfKey(e.ev.SuggestedMode)], Mode: modeToProto[modeOfKey(e.ev.RollMode)],
		ByMaster: true, Requested: true, Approved: e.ev.RollMode == e.ev.Requested,
	}
}

// dressDamage adds the parts and the steps to the damage of an attack, for whoever may
// have them. A Divine Smite die that did not count is the master's alone.
func (e *logEntry) dressDamage(d *playv1.CombatLogDamage, v combatViewer, dice, targetsOwn, holdsHitPoints bool) {
	if d == nil || e.dmg == nil {
		return
	}
	if dice {
		e.dressParts(d, v)
	}
	if !v.master && !targetsOwn && holdsHitPoints {
		e.dressShownAmount(d)
	}
	if v.master || targetsOwn {
		e.dressSteps(d, v.master)
	}
	if e.removed != nil && e.applied == nil && (v.master || targetsOwn) {
		d.Amount = e.removed.Amount
	}
}

// dressParts adds what each part rolled to the damage. The line of the part the master
// took out shows it as not counted.
func (e *logEntry) dressParts(d *playv1.CombatLogDamage, v combatViewer) {
	for _, r := range e.dmg.Parts {
		if e.removed != nil && r.Key == e.removed.Key {
			r.Counted = false
		}
		if r.Conditional && !v.master {
			r.Counted = true // the same lines whatever the target is
		}
		pr := partRollProto(r, dtPT)
		if v.master {
			pr.RiderPt = r.Rider
		}
		d.Parts = append(d.Parts, pr)
	}
}

// dressShownAmount gives a player who is not the target's the damage as rolled, before the
// target's modifiers and with every die the roll made.
func (e *logEntry) dressShownAmount(d *playv1.CombatLogDamage) {
	switch {
	case len(e.dmg.Parts) > 0:
		shown := slices.Clone(e.dmg.Parts)
		for i := range shown {
			if e.removed != nil && shown[i].Key == e.removed.Key {
				shown[i].Counted = false
			}
		}
		d.Amount = clamp32(shownTotalOf(shown), 0, math.MaxInt32)
	case len(e.dmg.Steps) > 0 || e.dmg.Shown > 0:
		d.Amount = e.dmg.Shown
	}
}

// dressSteps adds the steps resistance took the damage through, minus the sources the
// master told the app to ignore.
func (e *logEntry) dressSteps(d *playv1.CombatLogDamage, master bool) {
	names := func(string) string { return "" }
	if e.rolls != nil && e.rolls.names != nil {
		names = e.rolls.names
	}
	var ignored []string
	if e.applied != nil {
		ignored = e.applied.Ignored
	}
	d.Steps = stepsProto(e.dmg.Steps, ignored, names, dtPT, master)
}

// removalView is the master's removal of an extra, as a line for him and the attacker's player.
func (e *logEntry) removalView(v combatViewer, owner bool, out *playv1.CombatLogEntry) bool {
	if !v.master && !owner {
		return false
	}
	out.Reason = e.rolls.reasonOf(e.ev)
	out.KeyNamePt = e.partLabel
	return true
}
