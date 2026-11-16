package play

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/advantage"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/mountsave"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// The saving throw of a rider (SRD 5.1, "Mounted Combat"): if an effect moves the mount against its will while you ride it, or you are
// knocked prone while mounted, you make a DC 10 Dexterity saving throw or fall off, landing prone in a space within 5 feet of the
// mount. The question is a reaction window of its own kind (`mount_save`) that the rider's player or the master answers with RollMountSave
// (combat_mount_save_roll.go): it uses nobody's reaction and holds the turn like a concentration save. Encounter.mount_save is how it
// reads, and no other list carries it.
//
// What asks it, by the path that writes the square or the condition (the map of the writers is `carryRider`'s call sites):
//   - pusher.push: the push of a spell or a feature (Thunderwave, the Open Hand technique) when the mount really moves;
//   - shovePushes: the shove of a mount that pushes it; a shove of a rider is never a push (PUSH_BLOCKED);
//   - dragTo: a grappler's drag of the mount it holds;
//   - shoveKnocksProne, riderConsequence, giveCondition and giveTrapConditions: the rider knocked prone (a shove, a hit's rider, a spell
//     or a zone, a trap);
//
// and what does not: the mount's own move (its rider's player, or the master), the master's token, a teleport (the spell moves its
// caster, so the mount's own or the rider's, which is refused), the return of a mover to the square it left the reach at, the master's
// manual edit of the conditions and every undo.

// mountSaveDC is the DC of the saving throw, the SRD's 10.
const mountSaveDC = 10

// openMountSave opens the save of a rider that sits on the mount, unless it owes one already (one pending save for each rider, whatever
// asks) or it is defeated (a dead rider is no rider's fall). A rider at 0 hit points that is alive is unconscious and auto-fails a
// Dexterity saving throw (SRD 5.1, Conditions): it is thrown at once, with no window and no d20. The window keeps the square the mount
// was pushed to, so that the undo of the push, which puts the mount back, closes it.
func (s *Service) openMountSave(ctx context.Context, c *combatTx, _ []playdb.Combatant, rider, mount playdb.Combatant, cause playv1.MountSaveCause) error {
	if rider.Defeated {
		return nil
	}
	down, err := s.isDown(ctx, c.tx, c.session.CampaignID, rider)
	if err != nil {
		return err
	}
	if down {
		return s.throwDownRider(ctx, c, rider, mount, cause)
	}
	open, err := openWindowsOf(ctx, c)
	if err != nil {
		return err
	}
	if i := slices.IndexFunc(open, func(w playdb.ReactionWindow) bool {
		return w.Kind == string(reaction.MountSave) && conv.OrZero(w.ReactorID) == rider.ID
	}); i >= 0 {
		return s.carrySaveOn(ctx, c, open[i], mount, cause) // one save covers both
	}
	trigger := windowTrigger{Target: mount.ID, Cause: mountsave.Key(cause)}
	if placed(mount) {
		trigger.AtCol, trigger.AtRow = conv.OrZero(mount.GridCol), conv.OrZero(mount.GridRow)
	}
	_, err = s.openWindows(ctx, c, newGroup(), nil, []windowSpec{{kind: reaction.MountSave, reactor: &rider, trigger: trigger}})
	return err
}

// carrySaveOn keeps the open save of a rider when a second push moves its mount before it is answered: the window takes the new square
// (the one it had stays as a square the undo of the last push may put the mount back on), and a push makes the window the push's, so
// that the rider standing up does not end it. A prone after the push changes nothing.
func (s *Service) carrySaveOn(ctx context.Context, c *combatTx, w playdb.ReactionWindow, mount playdb.Combatant, cause playv1.MountSaveCause) error {
	if cause != playv1.MountSaveCause_MOUNT_SAVE_CAUSE_PUSHED || !placed(mount) {
		return nil
	}
	t := windowTriggerOf(w)
	here := [2]int32{conv.OrZero(mount.GridCol), conv.OrZero(mount.GridRow)}
	if t.Cause != mountsave.Prone && here == [2]int32{t.AtCol, t.AtRow} {
		return nil
	}
	if t.Cause != mountsave.Prone {
		t.Before = append(t.Before, [2]int32{t.AtCol, t.AtRow})
	}
	t.Cause, t.AtCol, t.AtRow = mountsave.Key(cause), here[0], here[1]
	body, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("encode the window's trigger: %w", err)
	}
	if _, err := c.q.SetReactionWindowTrigger(ctx, playdb.SetReactionWindowTriggerParams{ID: w.ID, Trigger: body}); err != nil {
		return fmt.Errorf("move a mount save on: %w", err)
	}
	return nil
}

// throwDownRider is the fall of a rider at 0 hit points: the same fall and landing as a failed save, written as an automatic failure
// with no d20 and no window.
func (s *Service) throwDownRider(ctx context.Context, c *combatTx, rider, mount playdb.Combatant, cause playv1.MountSaveCause) error {
	ev := actionEvent{Round: c.enc.Round, Secret: rider.Hidden || mount.Hidden, Actor: rider.ID, Target: mount.ID}
	ev.Reaction = &reactionEvent{Kind: string(reaction.MountSave), SaveDC: mountSaveDC, Cause: mountsave.Key(cause), AutoFail: true}
	return s.throwRider(ctx, c, rider, mount, &ev)
}

// mountPushed asks the rider's save when the mount an effect moved against its will carries one: it runs after the square of the mount
// and of its rider is written.
func (s *Service) mountPushed(ctx context.Context, c *combatTx, mountID string) error {
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	mount, ok := combatantByID(cs, mountID)
	rider := riderOf(cs, mountID)
	if !ok || rider == nil {
		return nil
	}
	return s.openMountSave(ctx, c, cs, *rider, mount, playv1.MountSaveCause_MOUNT_SAVE_CAUSE_PUSHED)
}

// riderProne asks the rider's save when the combatant an effect knocked prone sits on a mount: it runs after the condition is written.
func (s *Service) riderProne(ctx context.Context, c *combatTx, riderID string) error {
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return err
	}
	rider, ok := combatantByID(cs, riderID)
	if ok && rider.MountedOnID == nil { // the combatant knocked prone may be the mount: its rider owes a dismount
		return s.mountKnockedProne(ctx, c, cs, rider)
	}
	if !ok {
		return nil
	}
	mount := ridingOn(cs, rider)
	if mount == nil {
		return nil
	}
	return s.openMountSave(ctx, c, cs, rider, *mount, playv1.MountSaveCause_MOUNT_SAVE_CAUSE_PRONE)
}

// mountSaveGone says what a window asked is not so any more: the rider or the mount left the combat, the rider is not on that mount, the
// mount is not on the square it was pushed to (the push was undone), or the rider is not prone (the prone was undone or taken off).
func mountSaveGone(w playdb.ReactionWindow, t windowTrigger, byID map[string]playdb.Combatant) bool {
	rider, ok := byID[conv.OrZero(w.ReactorID)]
	mount, there := byID[t.Target]
	if !ok || !there || rider.Defeated || rider.MountedOnID == nil || *rider.MountedOnID != mount.ID {
		return true
	}
	if t.Cause == mountsave.Prone {
		return !slices.Contains(rider.Conditions, condProne)
	}
	if !placed(mount) {
		return true
	}
	at := [2]int32{conv.OrZero(mount.GridCol), conv.OrZero(mount.GridRow)}
	return at != [2]int32{t.AtCol, t.AtRow} && !slices.Contains(t.Before, at)
}

// mountSaveWait adds a rider's save to the line the caller reads. The master reads every one and a player only the character they see
// sit on a mount they see: a hidden mount or rider makes the wait the master's (RN-10).
func (wv *windowView) mountSaveWait(w playdb.ReactionWindow, reactor *playdb.Combatant, answers bool, wait *reaction.Wait) {
	player := reactor != nil && reactor.Kind == kindPlayer
	if wv.v.master {
		if player {
			wait.Savers = append(wait.Savers, reactor.Label)
		} else {
			wait.Self = true
		}
		return
	}
	if answers && !isCreature(*reactor) { // a creature's owner waits for the master
		return
	}
	mount, known := wv.byID[windowTriggerOf(w).Target]
	if player && wv.v.sees(*reactor) && known && wv.v.sees(mount) {
		wait.Savers = append(wait.Savers, reactor.Label)
		return
	}
	wait.Master = true
}

// mountSaveView builds Encounter.mount_save for the caller: the master and the player of a character that rides read it, nobody else (a
// creature's save is the master's).
func (s *Service) mountSaveView(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer) (*playv1.MountSavePrompt, error) {
	if d.enc.Status != statusActive || !slices.ContainsFunc(d.cs, func(c playdb.Combatant) bool { return c.MountedOnID != nil }) {
		return nil, nil
	}
	open, err := s.queries.ListOpenReactionWindows(ctx, d.enc.ID)
	if err != nil {
		return nil, s.dbError(ctx, "list the reaction windows", err)
	}
	byID := combatantsByID(d.cs)
	for _, w := range open {
		if w.Kind != string(reaction.MountSave) {
			continue
		}
		rider, ok := byID[conv.OrZero(w.ReactorID)]
		if !ok || (!v.master && (!v.owns(rider) || isCreature(rider))) { // a creature's save is the master's to answer
			continue
		}
		return s.mountSavePrompt(ctx, m, d, v, w, rider)
	}
	return nil, nil
}

// mountSavePrompt is the prompt of an open window: the rider, the mount when the caller sees it, the DC, the d20's circumstances and bonus.
func (s *Service) mountSavePrompt(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, w playdb.ReactionWindow, rider playdb.Combatant) (*playv1.MountSavePrompt, error) {
	t := windowTriggerOf(w)
	out := &playv1.MountSavePrompt{RiderId: rider.ID, Cause: mountsave.CauseOf(t.Cause)}
	if v.master || v.showSaveDC { // the table's rule (RN-24): the player reads the DC only when it shows it
		out.Dc = mountSaveDC
	}
	if mount, ok := combatantByID(d.cs, t.Target); ok && v.sees(mount) {
		out.MountId = mount.ID
	}
	facts, err := s.mountSaveFacts(ctx, nil, s.queries, m.CampaignID, d.enc, rider, statesOf(d.states))
	if err != nil {
		return nil, err
	}
	out.Mode, out.AutoFail = advantage.ModeToProto[facts.mode], facts.autoFail
	save, err := s.saveOf(ctx, nil, m.CampaignID, rider, "dex")
	if err != nil {
		return nil, s.dbError(ctx, "read a saving throw", err)
	}
	save = withRollBonus(save, statesOf(d.states), rider.ID, rules.RollAppliesSave) // the number RollMountSave adds
	out.SaveBonus, out.BonusKnown = conv.Int32(save.Bonus), save.Known
	return out, nil
}

// mountSaveFacts are the circumstances of the save a rider makes.
type mountSaveFacts struct {
	mode     combat.RollMode
	autoFail bool
	creature combat.Creature
	traits   link.Traits
}

// mountSaveFacts reads the circumstances of the rider's Dexterity saving throw: the mode with the sources the sheet shows for it and the
// automatic failure of a paralyzed, stunned, unconscious or petrified creature. tx is nil for a read through the pool.
func (s *Service) mountSaveFacts(ctx context.Context, tx pgx.Tx, q *playdb.Queries, campaignID string, enc playdb.Encounter, rider playdb.Combatant, states map[string][]playdb.CombatantState) (mountSaveFacts, error) {
	sheet, err := s.sheetOf(ctx, tx, campaignID, rider)
	if err != nil {
		return mountSaveFacts{}, fmt.Errorf("read the rider's sheet: %w", err)
	}
	sun, err := s.sunlightOf(ctx, q, enc)
	if err != nil {
		return mountSaveFacts{}, err
	}
	creature := sun.facts(creatureFacts(rider, states, sheet.Traits), rider)
	sources := combat.SaveMode(combat.SaveScene{Creature: creature, Ability: "dex", EffectVisible: true})
	return mountSaveFacts{mode: combat.Resolve(sources), autoFail: combat.AutoFailsSave(creature, "dex"), creature: creature, traits: sheet.Traits}, nil
}

// mountSaveHidden says a player who asks is not to be told whose save holds the turn: the rider or its mount is hidden from them (the
// master hid it, or the fog of the map does). The master is told whatever the window.
func mountSaveHidden(ctx context.Context, c *combatTx, w playdb.ReactionWindow) (bool, error) {
	if c.master {
		return false, nil
	}
	cs, err := listCombatants(ctx, c)
	if err != nil {
		return false, err
	}
	rider, ok := combatantByID(cs, conv.OrZero(w.ReactorID))
	mount, there := combatantByID(cs, windowTriggerOf(w).Target)
	if !ok || !there {
		return true, nil
	}
	v := c.viewer(authz.Membership{UserID: c.actorUserID, Role: authz.RolePlayer}, cs)
	return !v.sees(rider) || !v.sees(mount), nil
}

// mountSaveLine is the handler of a rider's roll: a line when the rider held ("Toren se segurou"). A rider that fell has the line of its
// fall instead (eventRiderThrown), written before this event.
func mountSaveLine(_ *logBuilder, _ playdb.ListEncounterEventsRow, entry *logEntry) bool {
	if entry.ev.Reaction == nil || !entry.ev.Reaction.Kept {
		return false
	}
	entry.kind = playv1.CombatLogKind_COMBAT_LOG_KIND_MOUNT_SAVE
	return true
}

// fillMountSave writes the line of a rider's save or fall: the cause and the outcome for everyone who gets the line, the d20, the total
// and the DC for the master and the rider's player (RN-10: another player reads only the outcome), and the master's note that no
// square next to the mount was free.
func (e *logEntry) fillMountSave(c *logViewing, out *playv1.CombatLogEntry) bool {
	re := e.ev.Reaction
	if re == nil {
		return false
	}
	ms := &playv1.CombatLogMountSave{
		Cause: mountsave.CauseOf(re.Cause), Held: re.Kept, AutoFail: re.AutoFail, LandedCol: re.LandCol, LandedRow: re.LandRow,
	}
	if c.v.master || (c.v.owns(c.actor) && !isCreature(c.actor)) {
		ms.D20, ms.Total = e.ev.D20, e.ev.Total
		if c.v.master || c.v.showSaveDC { // the table's rule (RN-24): the player reads the DC only when it shows it
			ms.Dc = re.SaveDC
		}
	}
	ms.NoRoomNextToMount = c.v.master && re.NoRoom
	out.MountSave = ms
	return true
}
