package play

import (
	"context"
	"fmt"
	"slices"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"

	"github.com/google/uuid"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/reaction"
)

// The saving throw a zone asks of a player's character, and the damage of a zone with no saving throw. The
// question is the one an effect asks (RollEffectSave, lasting_effects_save.go): an EffectSave reaction window
// whose trigger names the zone. The player rolls in the app or types the d20 as the table's dice say, the app
// adds the bonus, the turn waits for the answer, and what the save does (the damage, halved on a pass, or the
// restraint of the web) is done as the zone does it for an NPC. NPCs are rolled by the server at once, as the
// saving throws of a cast are.

// zoneWindowGroup is the group of the questions a creature owes on a turn: they are answered in the order
// they were opened, which is the order the spells were cast (the windows of a group are answered one at a time).
func zoneWindowGroup(encounterID, reactorID, mark string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("zone|"+encounterID+"|"+reactorID+"|"+mark)).String()
}

// askZoneSave opens the question of the saving throw to the creature's player.
func (s *Service) askZoneSave(ctx context.Context, c *combatTx, k zoneCatch, owner playdb.Combatant) error {
	trigger := windowTrigger{Actor: owner.ID, Target: k.who.ID, Key: k.z.row.SpellKey, Zone: k.z.row.ID, Phase: k.why}
	if k.made != nil { // a catch at the cast: the cast records the creature, with the conditions the answer may change
		h := castHit{Target: k.who.ID}
		if k.z.def.kind == zoneProne {
			h.CondSet, h.CondBefore = true, slices.Clone(k.who.Conditions)
		}
		k.made.Hits = append(k.made.Hits, h)
	}
	_, err := s.openWindows(ctx, c, zoneWindowGroup(c.enc.ID, k.who.ID, k.mark), nil, []windowSpec{{kind: reaction.EffectSave, reactor: &k.who, trigger: trigger}})
	return err
}

// catchWithoutSave is a zone whose spell asks no saving throw: the damage waits to be rolled, by whoever rolls the
// caster's damage (the caster's player, or the master for an NPC caster), and lands as every damage does.
func (s *Service) catchWithoutSave(ctx context.Context, c *combatTx, k zoneCatch, owner playdb.Combatant) error {
	f, ok := rules.ParseDice(k.z.row.DamageDice)
	if !ok || f.Count < 1 {
		return nil
	}
	p, err := c.q.InsertPendingDamage(ctx, playdb.InsertPendingDamageParams{
		EncounterID: c.enc.ID, AttackerID: &owner.ID, TargetID: k.who.ID, AttackKey: k.z.row.SpellKey, Status: pendingAwaitingRoll,
		DiceCount: clamp32(f.Count, 0, 100), DiceSides: clamp32(f.Sides, 0, 100), DiceBonus: clamp32(f.Bonus, -1000, 1000), DamageType: k.z.row.DamageType,
		CreatedAt: c.now,
	})
	if err != nil {
		return fmt.Errorf("open the zone's damage: %w", err)
	}
	ev := actionEvent{
		Round: k.round, Secret: owner.Hidden || k.who.Hidden, Actor: owner.ID, Target: k.who.ID, Key: k.z.row.SpellKey, Pending: p.ID,
		Lasting: &lastingEvent{Key: k.z.row.SpellKey, Change: "damage", Targets: []string{k.who.ID}, Caster: owner.ID, Reason: k.why, DType: k.z.row.DamageType},
	}
	c.queueZone(eventZoneTriggered, ev)
	return nil
}

// zoneSaveStep is what a zone's save answer needs besides the effect save's own: the zone and the turn it
// caught the creature on.
type zoneSaveStep struct {
	z     *zone
	owner playdb.Combatant
}

// loadZoneSave reads the zone a window asks for and starts the event and the log line of the answer.
func (s *Service) loadZoneSave(ctx context.Context, c *combatTx, x *effectSaveStep) error {
	row, err := c.q.GetSpellZone(ctx, playdb.GetSpellZoneParams{ID: x.t.Zone, EncounterID: c.enc.ID})
	z, ok := readZone(row)
	if err != nil || !ok {
		return errNotYourTurnToAnswer() // the window closes with its zone: never reached
	}
	if x.cs, err = listCombatants(ctx, c); err != nil {
		return err
	}
	owner, _ := combatantByID(x.cs, z.row.OwnerID)
	x.zone = &zoneSaveStep{z: z, owner: owner}
	x.ability, x.dc = z.row.SaveAbility, z.row.SaveDc
	x.ev = actionEvent{Round: c.enc.Round, Secret: owner.Hidden || x.reactor.Hidden, Actor: owner.ID, Target: x.reactor.ID, Key: z.row.SpellKey}
	x.le = &lastingEvent{Key: z.row.SpellKey, Change: "saved", Targets: []string{x.reactor.ID}, Caster: owner.ID, Reason: x.t.Phase, Ability: x.ability, DC: x.dc}
	return nil
}

// settleZoneSave does what the answered save does to the creature: the damage, halved on a pass, or the restraint.
func (s *Service) settleZoneSave(ctx context.Context, c *combatTx, x *effectSaveStep, saved bool) error {
	if x.call.skip {
		return nil
	}
	k := zoneCatch{z: x.zone.z, who: x.reactor, why: x.t.Phase, round: c.enc.Round}
	dealt, err := s.applyZone(ctx, c, x.cs, k, x.zone.owner, saved, &castHit{Target: x.reactor.ID})
	if err != nil {
		return err
	}
	x.le.Amount, x.le.DType = dealt.amount, dealt.dtype
	if dealt.pending.ID != "" {
		x.ev.Pending = dealt.pending.ID
	}
	if dealt.landed {
		return s.afterDamage(ctx, c, x.reactor, damageLanded{key: k.z.row.SpellKey, pending: dealt.pending, hit: dealt.hit})
	}
	return nil
}

// closeZoneWindows closes the questions a zone had open: it ended.
func (s *Service) closeZoneWindows(ctx context.Context, c *combatTx, zoneID, reason string) error {
	open, err := openWindowsOf(ctx, c)
	if err != nil {
		return err
	}
	for _, w := range open {
		if reaction.Kind(w.Kind) != reaction.EffectSave || windowTriggerOf(w).Zone != zoneID {
			continue
		}
		if _, err := s.closeWindow(ctx, c, w, windowClosed, windowReasonOf(reason), windowOutcome{}, ""); err != nil {
			return err
		}
	}
	return nil
}

// closeCastZoneWindows closes the questions the zones of a cast had open: the cast is undone.
func (s *Service) closeCastZoneWindows(ctx context.Context, c *combatTx, castID string) error {
	zs, err := listZones(ctx, c)
	if err != nil {
		return err
	}
	for _, z := range zs {
		if z.row.CastID != castID {
			continue
		}
		if err := s.closeZoneWindows(ctx, c, z.row.ID, endByMaster); err != nil {
			return err
		}
	}
	return nil
}

// zoneSavePrompt is the question of a zone's saving throw for whoever answers it: the spell, the ability, what a
// failure does (the type of the damage, never the dice) and the roll. The DC is the master's and the caster's own
// player's.
func (wv *windowView) zoneSavePrompt(w playdb.ReactionWindow, reactor *playdb.Combatant, out *playv1.ReactionWindow) error {
	t := windowTriggerOf(w)
	p := &playv1.EffectSavePrompt{EffectId: t.Zone, ZoneId: t.Zone, ZoneReason: t.Phase, HandedToMaster: t.Handed, Skippable: wv.v.master}
	i := slices.IndexFunc(wv.d.zones, func(z *zone) bool { return z.row.ID == t.Zone })
	if i < 0 {
		out.Prompt = &playv1.ReactionWindow_EffectSave{EffectSave: p}
		return nil
	}
	z := wv.d.zones[i]
	p.SourceNamePt, p.Ability, p.AbilityNamePt = wv.names(z.row.SpellKey), z.row.SaveAbility, wv.names("ability:"+z.row.SaveAbility)
	if owner, ok := wv.byID[z.row.OwnerID]; (ok && (wv.v.master || wv.v.owns(owner))) || (reactor != nil && wv.v.saveDCFor(*reactor)) {
		p.Dc = new(z.row.SaveDc)
	}
	p.TextPt = zoneSaveText(z, wv.names)
	if z.row.DamageDice != "" {
		p.DamageTypeNamePt, p.HalfOnSave = wv.names(z.row.DamageType), true
	}
	if reactor != nil {
		// A zone is a spell's: its save is against magic and what the spell imposes, as an effect's is.
		if err := wv.reactorSave(p, *reactor, z.row.SaveAbility, effectAgainst(z.row.SpellKey, nil), false); err != nil {
			return err
		}
	}
	out.Prompt = &playv1.ReactionWindow_EffectSave{EffectSave: p}
	return nil
}

// zoneSaveText says what the save asks, without a number.
func zoneSaveText(z *zone, names func(string) string) string {
	ability := names("ability:" + z.row.SaveAbility)
	if z.def.kind == zoneProne {
		return "Teste de resistência de " + ability + ". Se falhar, você cai no chão."
	}
	if z.def.kind == zoneRestrain {
		return "Teste de resistência de " + ability + ". Se falhar, você fica contido até se soltar."
	}
	return "Teste de resistência de " + ability + ". Se falhar, você sofre dano de " + names(z.row.DamageType) + ". Se passar, metade do dano."
}
