package play

import (
	"context"
	"slices"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// What a combatant can do now (GetTurnOptions). The read finds the combatant as the caller sees the fog
// (RN-10: a player never learns of what they cannot see), works the options out and disables them with
// the reason the combatant cannot act, and lists, for each attack and spell, who it can target as the
// viewer sees them. The order of the reads matters: the fog is read before any option, so that nothing
// the viewer cannot see reaches an option, a target or a reason; the terrain and the roll modes come
// before the targets that use them. The golden of combat_turn_options_golden_test.go pins the result by
// viewer.

// turnOptionsCall is what GetTurnOptions reads from its request, checked.
type turnOptionsCall struct {
	encounterID, combatantID string
}

// readTurnOptionsCall reads the request, in the order the answers are given: the ids.
func readTurnOptionsCall(req *playv1.GetTurnOptionsRequest) (turnOptionsCall, error) {
	var call turnOptionsCall
	var err error
	if call.encounterID, err = parseCombatID(req.GetEncounterId(), "encounter"); err != nil {
		return call, err
	}
	call.combatantID, err = parseCombatID(req.GetCombatantId(), "combatant")
	return call, err
}

// GetTurnOptions implements playv1connect.CombatServiceHandler.
func (s *Service) GetTurnOptions(
	ctx context.Context,
	req *connect.Request[playv1.GetTurnOptionsRequest],
) (*connect.Response[playv1.GetTurnOptionsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	call, err := readTurnOptionsCall(req.Msg)
	if err != nil {
		return nil, err
	}
	d, v, sight, who, err := s.locateTurnOptions(ctx, m, call)
	if err != nil {
		return nil, err
	}
	reads := s.turnReadsOf(m.CampaignID, who)
	opts, code, err := s.workOutOptions(ctx, m, d, v, who, reads)
	if err != nil {
		return nil, err
	}
	res, terrain, err := s.startTurnOptions(ctx, m, d, v, sight, who, opts)
	if err != nil {
		return nil, err
	}
	modes, err := s.weighTurnModes(ctx, m, d, v, sight, who)
	if err != nil {
		return nil, err
	}
	listAttackTargets(res, d, v, who, terrain, modes)
	if err := s.listSpellTargets(ctx, m, res, d, v, who, terrain, modes); err != nil {
		return nil, err
	}
	if err := s.listPendingDamage(ctx, m, res, d, v, who); err != nil {
		return nil, err
	}
	if res.ManeuverDice, err = s.listManeuverDice(ctx, who, reads); err != nil {
		return nil, err
	}
	res.ContestAttackOptions, res.ContestState, err = s.contestOptionsFor(ctx, m, d, v, who, opts, code)
	if err != nil {
		return nil, s.dbError(ctx, "work out the contests of the turn", err)
	}
	if err := s.markUnseenTargets(ctx, m, res, d, v, sight, who); err != nil {
		return nil, err
	}
	return connect.NewResponse(res), nil
}

// markUnseenTargets marks, for the master reading the turn of a player's character, the targets
// the character's player does not see: they stay listed (the master knows them) but cannot be
// picked, as the server refuses them for the character (RollAttack, CastSpell).
func (s *Service) markUnseenTargets(ctx context.Context, m authz.Membership, res *playv1.GetTurnOptionsResponse, d *encounterData, v combatViewer, sight *fogSight, who playdb.Combatant) error {
	eyes, _, err := s.actorSightFor(ctx, m, d.enc, d.cs, v, sight, who)
	if err != nil {
		return s.dbError(ctx, "work out what the character sees", err)
	}
	if !v.master || eyes.master {
		return nil
	}
	var lists [][]*playv1.TargetInReach
	for _, a := range res.GetAttackTargets() {
		lists = append(lists, a.GetTargets())
	}
	for _, sp := range res.GetSpellTargets() {
		lists = append(lists, sp.GetTargets())
	}
	for _, r := range res.GetResourceTargets() {
		for _, t := range r.GetTargets() {
			lists = append(lists, []*playv1.TargetInReach{t.GetTarget()})
		}
	}
	for _, list := range lists {
		for _, t := range list {
			if c, ok := combatantByID(d.cs, t.GetCombatantId()); ok && !eyes.sees(c) {
				t.NotSeen, t.Untargetable = true, true
			}
		}
	}
	return nil
}

// locateTurnOptions finds the combatant as the caller sees the fog, and checks the caller may act for it.
func (s *Service) locateTurnOptions(ctx context.Context, m authz.Membership, call turnOptionsCall) (*encounterData, combatViewer, *fogSight, playdb.Combatant, error) {
	_, d, err := s.readEncounter(ctx, m.CampaignID, call.encounterID)
	if err != nil {
		return nil, combatViewer{}, nil, playdb.Combatant{}, err
	}
	v, sight, err := s.viewerWith(ctx, m, d.enc, d.cs)
	if err != nil {
		return nil, v, nil, playdb.Combatant{}, s.dbError(ctx, "work out what the player sees", err)
	}
	who, err := findCombatant(d.cs, call.combatantID, v)
	if err != nil {
		return nil, v, sight, who, err
	}
	return d, v, sight, who, v.mayAct(who)
}

// workOutOptions works out the options of the combatant, and disables them all with the reason it cannot
// act. It answers the reason too, which the contests read.
func (s *Service) workOutOptions(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, who playdb.Combatant, reads *turnReads) (*rulesv1.TurnOptions, rulesv1.DisabledReasonCode, error) {
	none := rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_UNSPECIFIED
	opts, err := s.optionsOf(ctx, nil, m.CampaignID, who)
	if err != nil {
		return nil, none, s.dbError(ctx, "work out the turn options", err)
	}
	if err := s.attachMetamagic(ctx, reads, who, opts); err != nil {
		return nil, none, s.dbError(ctx, "work out the Metamagic", err)
	}
	if err := s.attachTableOptions(ctx, reads, who, opts); err != nil {
		return nil, none, s.dbError(ctx, "work out the table's cast options", err)
	}
	code, err := s.gate(ctx, nil, m.CampaignID, d.enc, who)
	if err != nil {
		return nil, none, s.dbError(ctx, "check the combatant's turn", err)
	}
	if code != none {
		disableAll(opts, code)
		s.explainEffectReason(d, who, v, opts, code, s.namesFor(ctx, m.CampaignID))
	} else {
		explainSpellReasons(opts)
	}
	return opts, code, nil
}

// startTurnOptions reads the terrain the viewer plans on and the table's rules, and starts the answer. The
// cover a player is shown, and the targets it makes untargetable, come from the terrain they know and the
// creatures they see.
func (s *Service) startTurnOptions(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, sight *fogSight, who playdb.Combatant, opts *rulesv1.TurnOptions) (*playv1.GetTurnOptionsResponse, grid.Terrain, error) {
	terrain, err := s.terrainOf(ctx, nil, m.CampaignID, d.enc)
	if err != nil {
		return nil, grid.Terrain{}, s.dbError(ctx, "read the terrain", err)
	}
	known, err := sight.knownTerrain(ctx, nil, v)
	if err != nil {
		return nil, grid.Terrain{}, s.dbError(ctx, "read the terrain the player knows", err)
	}
	table, err := s.tableRules(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, grid.Terrain{}, s.dbError(ctx, "read the table's rules", err)
	}
	res := &playv1.GetTurnOptionsResponse{Options: opts, YourTurn: actsNow(d.enc, who), CriticalRule: criticalRuleProto(table)}
	s.effectTurnInfo(d, who, v, res, s.namesFor(ctx, m.CampaignID))
	return res, planOn(v, terrain, known), nil
}

// weighTurnModes works out the roll modes of the turn. The master's turn options carry no sight (they plan
// on the real terrain), but the mode of an attack depends on who the players see (an NPC they cannot see
// attacks with advantage, SRD 5.1): RollAttack reads it for the master too, so the suggestion must, or the
// typed d20 count is wrong.
func (s *Service) weighTurnModes(ctx context.Context, m authz.Membership, d *encounterData, v combatViewer, sight *fogSight, who playdb.Combatant) (turnModes, error) {
	modeSight := sight
	if modeSight == nil && v.master {
		var err error
		if modeSight, err = s.fogSightOf(ctx, m.CampaignID, d.enc); err != nil {
			return turnModes{}, s.dbError(ctx, "work out what the players see", err)
		}
	}
	modes, err := s.turnModes(ctx, m.CampaignID, d, modeSight, v, who)
	if err != nil {
		return turnModes{}, s.dbError(ctx, "work out the roll modes", err)
	}
	return modes, nil
}

// listAttackTargets lists who each attack can target.
func listAttackTargets(res *playv1.GetTurnOptionsResponse, d *encounterData, v combatViewer, who playdb.Combatant, terrain grid.Terrain, modes turnModes) {
	for _, a := range res.GetOptions().GetAttacks() {
		if a.GetAttack().GetSaveDc() > 0 {
			continue // a saving throw, not an attack roll: the spells list
		}
		targets := targetsFor(terrain, d.cs, who, v, reachFt(a.GetAttack().GetRangeFt(), a.GetAttack().GetLongRangeFt()), isTheatre(d.enc))
		modes.annotate(a.GetAttack().GetKey(), targets)
		modes.dropIgnoredCover(a.GetAttack().GetKey(), targets)
		res.AttackTargets = append(res.AttackTargets, &playv1.AttackTargets{AttackKey: a.GetAttack().GetKey(), Targets: targets})
	}
}

// listSpellTargets lists who each spell can target, and the targets of the resources.
func (s *Service) listSpellTargets(ctx context.Context, m authz.Membership, res *playv1.GetTurnOptionsResponse, d *encounterData, v combatViewer, who playdb.Combatant, terrain grid.Terrain, modes turnModes) error {
	scene := spellTargetScene{terrain: terrain, who: who, cs: d.cs, v: v, theatre: isTheatre(d.enc), modes: modes}
	var err error
	if res.SpellTargets, err = s.spellTargetsFor(ctx, m.CampaignID, scene, res.GetOptions()); err != nil {
		return s.dbError(ctx, "work out the spell targets", err)
	}
	res.ResourceTargets = resourceTargetsFor(terrain, d.cs, who, v, res.GetOptions(), d.enc)
	return nil
}

// listPendingDamage lists the damage the combatant's hits left waiting, the ones the viewer may know of (RN-10).
func (s *Service) listPendingDamage(ctx context.Context, m authz.Membership, res *playv1.GetTurnOptionsResponse, d *encounterData, v combatViewer, who playdb.Combatant) error {
	open, err := s.queries.ListOpenPendingDamages(ctx, d.enc.ID)
	if err != nil {
		return s.dbError(ctx, "list the pending damage", err)
	}
	for _, p := range open {
		if conv.OrZero(p.AttackerID) == who.ID && pendingVisible(p, d.cs, v) {
			res.PendingDamages = append(res.PendingDamages, s.pendingView(ctx, m.CampaignID, p, d.cs, v))
		}
	}
	return nil
}

// listManeuverDice lists the weapon-attack table dice of a player's character (the ones RollAttack takes as
// maneuver_key), each with the uses left of its resource pool; a die with none left stays listed. Nothing for
// an NPC.
func (s *Service) listManeuverDice(ctx context.Context, who playdb.Combatant, reads *turnReads) ([]*playv1.ManeuverDieOption, error) {
	if who.Kind != kindPlayer || !backedByCharacter(who) {
		return nil, nil
	}
	sheet, err := reads.sheetOf(ctx)
	if err != nil {
		return nil, s.dbError(ctx, "read the character's maneuver dice", err)
	}
	dice := slices.DeleteFunc(slices.Clone(sheet.Traits.Maneuvers), func(m link.Maneuver) bool {
		return !maneuverFits(m, rules.ManeuverAttack, "", "")
	})
	if len(dice) == 0 {
		return nil, nil
	}
	vit, err := reads.vitalsOf(ctx)
	if err != nil {
		return nil, s.dbError(ctx, "read the character's resources", err)
	}
	names := map[string]string{}
	for _, r := range vit.GetResources() {
		names[r.GetKey()] = r.GetNamePt()
	}
	out := make([]*playv1.ManeuverDieOption, 0, len(dice))
	for _, m := range dice {
		left, _ := poolLeft(vit, m.Resource)
		out = append(out, &playv1.ManeuverDieOption{
			Die: &rulesv1.RollDie{
				Key: m.Key, NamePt: m.NamePT, Applies: m.Applies, Sides: clamp32(m.Sides, 0, 100), Resource: m.Resource,
				Targets: m.Targets, Ability: m.Ability, TextPt: m.TextPT,
			},
			ResourceNamePt: names[m.Resource],
			UsesLeft:       max(left, 0),
		})
	}
	return out, nil
}

// disableAll disables every option with the reason: nothing can be done off
// turn, outside a running combat or by a combatant that is out.
func disableAll(o *rulesv1.TurnOptions, code rulesv1.DisabledReasonCode) {
	reason := func() *rulesv1.DisabledReason { return &rulesv1.DisabledReason{Code: code} }
	for _, a := range o.GetAttacks() {
		a.Enabled, a.Reason = false, reason()
	}
	for _, sp := range o.GetSpells() {
		sp.Enabled, sp.Reason = false, reason()
	}
	for _, a := range slices.Concat(o.GetStandardActions(), o.GetFeatureActions()) {
		a.Enabled, a.Reason = false, reason()
	}
}

// targetsFor lists who the combatant's attack of the given reach can target,
// as the viewer sees them: not itself, not a defeated one, not a hidden one
// for a player (RN-10), in turn order, each with the distance and whether it
// is too far. In a combat without a grid (theatre) there is no distance and
// nothing is too far: every combatant the rules allow is a target, and the master
// judges the reach (RN-25).
func targetsFor(terrain grid.Terrain, cs []playdb.Combatant, attacker playdb.Combatant, v combatViewer, reach int32, theatre bool) []*playv1.TargetInReach {
	pool := coverPool(cs, v)
	var out []*playv1.TargetInReach
	for _, t := range cs {
		if t.ID == attacker.ID || t.Defeated || !v.sees(t) {
			continue
		}
		cover := coverAgainst(terrain, attacker, t, pool)
		target := withCover(&playv1.TargetInReach{CombatantId: t.ID, Label: t.Label, State: stateOf(t)}, cover)
		target.Untargetable = cover.total() && !v.master
		if theatre {
			out = append(out, target)
			continue
		}
		if dist, ok := distanceFt(attacker, t); ok {
			target.DistanceFt = &dist
			target.TooFar = dist > reach
		} else {
			target.TooFar = !v.master // a player cannot attack what has no square (RollAttack)
		}
		out = append(out, target)
	}
	return out
}

// withCover puts the cover a target has against the attacker on its list entry.
func withCover(t *playv1.TargetInReach, cv coverView) *playv1.TargetInReach {
	t.Cover, t.CoverSource = coverDegreeProto(cv.key()), cv.source
	return t
}
