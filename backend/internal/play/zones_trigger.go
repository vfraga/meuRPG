package play

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// What a zone does to the creature it catches: the saving throw and the damage of the spell, through
// the paths the cast and the effects that last already use. The creature makes the spell's saving
// throw as a cast makes it for a target (spellSave); the damage is a pending damage like the one an
// effect's trigger opens (an NPC takes it at once, a player's character waits for the master), halved
// when the save is passed, and the concentration check of the damage runs as for any damage.

// The reasons a zone caught a creature, as the event keeps them.
const (
	zoneWhyEnter = "zone_enter"
	zoneWhyStart = "zone_start"
	zoneWhyCast  = "zone_cast" // standing in the area when the spell was cast
	zoneWhyEnd   = "zone_end"  // ended its turn inside
)

// eventZoneTriggered is a creature caught by the area a spell keeps on the map, and eventZoneMoved
// the area changing place (Moonbeam moved, Cloudkill drifting); both carry a lastingEvent.
const (
	eventZoneTriggered = "spell_zone_triggered"
	eventZoneMoved     = "spell_zone_moved"
)

// zoneCatch is one creature a zone catches now.
type zoneCatch struct {
	z     *zone
	who   playdb.Combatant
	why   string
	mark  string
	round int32
	// made is the event of the cast the catch belongs to (a catch at the cast): what the catch changes is
	// written into it, so that the undo of the cast puts it back. nil for a catch outside a cast.
	made *actionEvent
}

// spares says the zone leaves the creature alone: the caster and the caster's allies for the zones
// that spare them.
func (z *zone) spares(owner, who playdb.Combatant) bool {
	return z.def.sparesAllies && (who.ID == owner.ID || who.Side == owner.Side)
}

// asks says the zone catches a creature for this reason: it was inside at the cast, it came in, it began
// its turn inside, or it ended its turn inside.
func (z *zone) asks(why string) bool {
	switch why {
	case zoneWhyStart:
		return z.def.onStart
	case zoneWhyCast:
		return z.def.onCast
	case zoneWhyEnd:
		return z.def.onTurnEnd
	}
	return z.def.onEnter
}

// catches reports whether the creature is one the zone can catch: on the map, standing, not spared
// and not yet hit on this turn.
func (z *zone) catches(owner, who playdb.Combatant, mark string) bool {
	return placed(who) && !who.Defeated && !z.spares(owner, who) && !z.hitOn(who.ID, mark)
}

// catchZone catches a creature in a zone: it writes that the zone hit it on this turn, makes its saving
// throw and deals what the spell says.
func (s *Service) catchZone(ctx context.Context, c *combatTx, cs []playdb.Combatant, k zoneCatch) error {
	owner, ok := combatantByID(cs, k.z.row.OwnerID)
	if !ok || !k.z.asks(k.why) || !k.z.catches(owner, k.who, k.mark) {
		return nil
	}
	if k.z.def.kind == zoneRestrain {
		held, err := s.heldByZone(ctx, c, k.z, owner, k.who)
		if err != nil || held {
			return err // a creature the web holds or has a save for already is not asked twice
		}
	}
	if err := k.z.markHit(ctx, c, k.who.ID, k.mark); err != nil {
		return err
	}
	if k.z.def.slows {
		if err := s.slowInZone(ctx, c, cs, k.z, owner, k.who); err != nil {
			return err
		}
	}
	switch {
	case k.z.row.SaveAbility == "":
		return s.catchWithoutSave(ctx, c, k, owner)
	case k.who.Kind == kindPlayer:
		return s.askZoneSave(ctx, c, k, owner) // a player's character is asked (zones_save.go)
	}
	return s.resolveCatch(ctx, c, cs, k, owner)
}

// resolveCatch is the saving throw of a creature the zone caught, what it brings and its line.
func (s *Service) resolveCatch(ctx context.Context, c *combatTx, cs []playdb.Combatant, k zoneCatch, owner playdb.Combatant) error {
	hit, err := s.zoneSave(ctx, c, cs, k.z, owner, k.who)
	if err != nil {
		return err
	}
	dealt, err := s.applyZone(ctx, c, cs, k, owner, hit.Save != nil && hit.Save.Saved, &hit)
	if err != nil {
		return err
	}
	if k.made != nil { // a catch at the cast: the cast records every creature it caught, whatever its save
		k.made.Hits = append(k.made.Hits, hit)
	}
	writeZoneCaught(c, k, owner, hit, dealt)
	if dealt.landed {
		return s.afterDamage(ctx, c, k.who, damageLanded{key: k.z.row.SpellKey, pending: dealt.pending, hit: dealt.hit})
	}
	return nil
}

// applyZone does what the zone does to a creature that made its save or did not: the damage of the
// spells that burn, the restraint of the web.
func (s *Service) applyZone(ctx context.Context, c *combatTx, cs []playdb.Combatant, k zoneCatch, owner playdb.Combatant, saved bool, h *castHit) (zoneDamageDealt, error) {
	switch {
	case k.z.def.kind == zoneDamage:
		return s.dealZoneDamage(ctx, c, k, saved)
	case saved:
	case k.z.def.kind == zoneProne:
		return zoneDamageDealt{}, s.knockProne(ctx, c, k, h)
	default:
		return zoneDamageDealt{}, s.restrainInZone(ctx, c, cs, k, owner)
	}
	return zoneDamageDealt{}, nil
}

// zoneSave makes the creature's saving throw against the zone's spell, as a cast does for a target.
func (s *Service) zoneSave(ctx context.Context, c *combatTx, cs []playdb.Combatant, z *zone, owner, who playdb.Combatant) (castHit, error) {
	m := authz.Membership{CampaignID: c.session.CampaignID, Role: authz.RoleMaster}
	modes, err := s.spellModes(ctx, c, m, combatViewer{master: true}, cs, owner, nil)
	if err != nil {
		return castHit{}, err
	}
	sp := link.Spell{Key: z.row.SpellKey, SaveAbility: z.row.SaveAbility, SaveOnSuccess: "half", SaveDC: int(z.row.SaveDc)}
	hit := castHit{Target: who.ID}
	if err := s.spellSave(ctx, c, m, sp, owner, who, coverView{}, &hit, modes); err != nil {
		return castHit{}, err
	}
	return hit, nil
}

// zoneDamageDealt is the damage a zone dealt to a creature.
type zoneDamageDealt struct {
	amount  int32
	dtype   string
	pending playdb.PendingDamage
	hit     damageHit
	landed  bool
}

// dealZoneDamage rolls the damage of the zone, halves it for a creature that passed its save and lands
// it as the damage of an effect lands.
func (s *Service) dealZoneDamage(ctx context.Context, c *combatTx, k zoneCatch, saved bool) (zoneDamageDealt, error) {
	f, ok := rules.ParseDice(k.z.row.DamageDice)
	if !ok || f.Count < 1 {
		return zoneDamageDealt{}, nil
	}
	roll, err := dice.Roll(s.roller, dice.Expr{Count: f.Count, Sides: f.Sides, Modifier: f.Bonus})
	if err != nil {
		return zoneDamageDealt{}, fmt.Errorf("roll a zone's damage: %w", err)
	}
	total := roll.Total
	if saved {
		total /= 2
	}
	amount, err := s.afterResistance(ctx, c, k.who, k.z.row.DamageType, clamp32(total, 0, math.MaxInt32))
	if err != nil {
		return zoneDamageDealt{}, err
	}
	status, resolved := pendingRolled, (*time.Time)(nil)
	if holdsHP(k.who) {
		status, resolved = pendingApplied, &c.now
	}
	p, err := c.q.InsertEffectPendingDamage(ctx, playdb.InsertEffectPendingDamageParams{
		EncounterID: c.enc.ID, TargetID: k.who.ID, Status: status, DiceCount: clamp32(f.Count, 0, 100), DiceSides: clamp32(f.Sides, 0, 100),
		DiceBonus: clamp32(f.Bonus, -1000, 1000), DamageType: k.z.row.DamageType, Faces: faces32(roll.Faces), Amount: &amount,
		RollTotal: new(conv.Int32(roll.Total)), CreatedAt: c.now, ResolvedAt: resolved, EffectSourceKey: &k.z.row.SpellKey,
	})
	if err != nil {
		return zoneDamageDealt{}, fmt.Errorf("open the zone's damage: %w", err)
	}
	hit, _, err := s.landDamage(ctx, c, p, k.who, amount)
	if err != nil {
		return zoneDamageDealt{}, err
	}
	return zoneDamageDealt{amount: amount, dtype: k.z.row.DamageType, pending: p, hit: hit, landed: hit.Applied}, nil
}

// writeZoneCaught writes the line of a creature caught by a zone: why, the saving throw, and the damage.
func writeZoneCaught(c *combatTx, k zoneCatch, owner playdb.Combatant, hit castHit, dealt zoneDamageDealt) {
	l := &lastingEvent{Key: k.z.row.SpellKey, Change: "failed", Targets: []string{k.who.ID}, Caster: owner.ID, Reason: k.why, Ability: k.z.row.SaveAbility, Amount: dealt.amount, DType: dealt.dtype}
	if sv := hit.Save; sv != nil {
		l.D20, l.Total, l.DC = sv.D20, sv.Total, sv.DC
		if sv.Saved {
			l.Change = "saved"
		}
	}
	ev := actionEvent{
		Round: k.round, Secret: owner.Hidden || k.who.Hidden, Actor: owner.ID, Target: k.who.ID, Key: k.z.row.SpellKey,
		Pending: dealt.pending.ID, Lasting: l,
	}
	if dealt.landed {
		ev.Applied, ev.Before, ev.After = dealt.hit.Applied, dealt.hit.Before, dealt.hit.After
	}
	c.queueZone(eventZoneTriggered, ev)
}

// heldByZone says the zone's spell already holds the creature or keeps a saving throw for it: it carries an
// effect of this spell.
func (s *Service) heldByZone(ctx context.Context, c *combatTx, z *zone, owner, who playdb.Combatant) (bool, error) {
	rows, err := c.q.ListLastingEffects(ctx, c.enc.ID)
	if err != nil {
		return false, fmt.Errorf("list the effects: %w", err)
	}
	return slices.ContainsFunc(rows, func(st playdb.CombatantState) bool {
		key := conv.OrZero(st.SourceKey)
		return st.CombatantID == who.ID && conv.OrZero(st.SourceID) == owner.ID && (key == z.row.SpellKey || key == z.def.restrainKey)
	}), nil
}

// The effects of the web (combat_effects.json).
const (
	webRestrainedKey      = "effect:web-restrained"
	entangleRestrainedKey = "effect:entangle-restrained"
)

// knockProne knocks a creature prone: it stays so until it stands up. The condition it had before goes into h,
// for the undo.
func (s *Service) knockProne(ctx context.Context, c *combatTx, k zoneCatch, h *castHit) error {
	return s.giveCondition(ctx, c, k.who, condProne, h)
}

// restrainInZone restrains a creature that failed its save against the zone, with the zone's effect, as the
// failed save at the start of the turn of the web's effect does.
func (s *Service) restrainInZone(ctx context.Context, c *combatTx, cs []playdb.Combatant, k zoneCatch, owner playdb.Combatant) error {
	z, who := k.z, k.who
	content, err := s.contentOf(ctx, c)
	if err != nil {
		return err
	}
	key := z.def.restrainKey
	def, ok := content.CombatEffect(key)
	if !ok {
		return nil
	}
	dc := z.row.SaveDc
	made, err := s.addEffects(ctx, c, cs, effectSpec{
		key: key, sourceKind: "spell", def: def, caster: &owner, group: z.row.CastID, targets: []playdb.Combatant{who},
		dur: durationSpec{Kind: def.Duration.Kind}, concentration: z.row.Concentration, dc: &dc,
	})
	if err != nil {
		return err
	}
	if k.made != nil { // in a cast: the cast's own event keeps the effects, for its undo
		keepCastEffects(k.made, key, made)
		return nil
	}
	if k.why == zoneWhyCast { // the answer of a creature caught at the cast: the cast's undo takes the effect away by its group
		return nil
	}
	return s.addedEvent(ctx, c, cs, made, key)
}

// zoneLine is an event a zone leaves, held until the change's own event is written.
type zoneLine struct {
	kind string
	ev   actionEvent
}

// queueZone holds the event of a zone for the end of the change.
func (c *combatTx) queueZone(kind string, ev actionEvent) {
	c.zoneOut = append(c.zoneOut, zoneLine{kind: kind, ev: ev})
}

// flushZoneLines writes the events the zones left, in order, after the change's own.
func flushZoneLines(ctx context.Context, c *combatTx) error {
	lines := c.zoneOut
	c.zoneOut = nil
	for _, l := range lines {
		if err := insertEvent(ctx, c, l.kind, &c.actorUserID, nil, l.ev); err != nil {
			return err
		}
	}
	return nil
}

// spiritGuardiansSlowedKey is the effect that halves the speed of a creature the aura caught, for its turn.
const spiritGuardiansSlowedKey = "effect:spirit-guardians-slowed"

// slowInZone halves the speed of a creature the zone caught, until the end of its turn (SRD 5.1, Spirit
// Guardians: the speed is halved in the area; the app holds it for the turn the creature is caught on).
func (s *Service) slowInZone(ctx context.Context, c *combatTx, cs []playdb.Combatant, z *zone, owner, who playdb.Combatant) error {
	content, err := s.contentOf(ctx, c)
	if err != nil {
		return err
	}
	def, ok := content.CombatEffect(spiritGuardiansSlowedKey)
	if !ok {
		return nil
	}
	rows, err := c.q.ListLastingEffects(ctx, c.enc.ID)
	if err != nil {
		return fmt.Errorf("list the effects: %w", err)
	}
	if slices.ContainsFunc(rows, func(st playdb.CombatantState) bool {
		return st.CombatantID == who.ID && conv.OrZero(st.SourceKey) == spiritGuardiansSlowedKey
	}) {
		return nil
	}
	made, err := s.addEffects(ctx, c, cs, effectSpec{
		key: spiritGuardiansSlowedKey, sourceKind: "spell", def: def, caster: &owner, group: z.row.CastID, targets: []playdb.Combatant{who},
		dur: durationSpec{Kind: def.Duration.Kind},
	})
	if err != nil {
		return err
	}
	return s.addedEvent(ctx, c, cs, made, spiritGuardiansSlowedKey)
}

// keepCastEffects writes the effects a catch at the cast put on creatures into the event of the cast.
func keepCastEffects(made *actionEvent, key string, rows []playdb.CombatantState) {
	if made.Lasting == nil {
		made.Lasting = &lastingEvent{Key: key, Change: "added"}
	}
	for _, r := range rows {
		made.Lasting.Targets = append(made.Lasting.Targets, r.CombatantID)
		made.Lasting.Effects = append(made.Lasting.Effects, r.ID)
	}
}
