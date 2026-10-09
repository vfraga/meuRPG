package play

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// Who sees what in a combat (RN-10, RN-20). The master sees everything. A
// player sees:
//
//	a combatant   when it is not hidden
//	its numbers   (initiative roll, bonus, speed, economy) only for their own
//	              character and its creatures; an NPC's initiative total never
//	hit points    an NPC's as a word (CombatantState), never as numbers; a
//	              player's character only as the word "Caído" at 0; a
//	              creature's as numbers to its owner's player and the master,
//	              as a word to everyone else (MR-037)
//	the turn      "Vez do mestre" when a hidden combatant is on turn
//
// Everything a player may not see is left out of the response, never
// blanked: a hidden combatant's ID, label and position never leave the
// server. The same rules decide what each audience gets on the live stream.

// combatViewer is who reads a combat: the caller.
type combatViewer struct {
	master bool
	userID string
	// unseen are the NPC combatants the viewer does not see on a map with the fog of
	// war on (combat_fog.go): for them they are like hidden combatants. Nil on a map
	// without the fog, and for the master.
	unseen map[string]bool
	// sight is what the players see of the combat's map, for the viewer's own
	// questions (seesAt); nil without the fog and for the master.
	sight *fogSight
	// hideDeath is the table's rule "testes contra a morte só para o dono e o
	// mestre" (RN-24) as it applies to this viewer: the death saves of a
	// character that is not theirs are not theirs to see. Never set for the
	// master, and false when the table leaves them visible (the default).
	hideDeath bool
}

// deathHiddenFrom says whether the table hides this character's death saves
// from the viewer: the successes, the failures, each roll and every
// intermediate step. Its owner and the master always see them.
func (v combatViewer) deathHiddenFrom(c playdb.Combatant) bool {
	return v.hideDeath && !v.master && !v.owns(c)
}

// hiddenFrom is deathHiddenFrom for an event that was stamped when it was written:
// whether the table hid the death saves then, whatever it says now.
func (v combatViewer) hiddenFrom(stamped bool, c playdb.Combatant) bool {
	return stamped && !v.master && !v.owns(c)
}

// seesAt says whether the viewer sees the combatant standing on the square, as it
// stood there at some moment (an opportunity offer's square, a move's old one): a
// combatant they see now, or an NPC whose square they see, whatever it is now.
func (v combatViewer) seesAt(c playdb.Combatant, sq grid.Square) bool {
	if v.sees(c) {
		return true
	}
	return !c.Hidden && v.sight != nil && c.Kind == kindNPC && v.sight.sight.Sees(v.userID, sq)
}

// seesAtOffer is seesAt for the square an opportunity offer was made at. An offer
// the master made by hand (a combat without a map, RN-25) has no square: nobody
// stood anywhere, so the viewer sees the mover only if they see it now.
func (v combatViewer) seesAtOffer(c playdb.Combatant, o playdb.OpportunityOffer) bool {
	if o.LeftCol == nil || o.LeftRow == nil {
		return v.sees(c)
	}
	return v.seesAt(c, grid.Square{Col: int(*o.LeftCol), Row: int(*o.LeftRow)})
}

func viewerOf(m authz.Membership) combatViewer {
	return combatViewer{master: m.Role == authz.RoleMaster, userID: m.UserID}
}

// sees says whether the viewer sees the combatant.
func (v combatViewer) sees(c playdb.Combatant) bool {
	return v.master || (!c.Hidden && !v.unseen[c.ID])
}

// owns says whether the viewer's player plays the combatant: their own
// character, or one of its creatures (which carry the owner's user_id). Only a
// player's character is "mine" (Combatant.mine); controlled_by_me is this.
func (v combatViewer) owns(c playdb.Combatant) bool {
	return !v.master && c.UserID != nil && v.userID != "" && *c.UserID == v.userID
}

// encounterData is a combat as stored: the row and its combatants in turn
// order. The filter for a viewer is view.
type encounterData struct {
	enc playdb.Encounter
	cs  []playdb.Combatant
}

// loadEncounter reads a combat's combatants, in turn order.
func loadEncounter(ctx context.Context, q *playdb.Queries, enc playdb.Encounter) (*encounterData, error) {
	cs, err := q.ListCombatants(ctx, enc.ID)
	if err != nil {
		return nil, fmt.Errorf("list the combatants: %w", err)
	}
	return &encounterData{enc: enc, cs: cs}, nil
}

// turnView is the turn as one viewer sees it (RN-20, joint turns): who acts,
// whether it is the master's part, and the members of the group on turn the
// viewer is told about.
type turnView struct {
	currentID  string
	masterTurn bool
	groupIDs   []string
	// flags says whether the viewer gets each member's turn_part_ended.
	flags bool
}

// turnFor works out the turn for the viewer.
//
//   - The master sees the whole group, and its first member who acts is the
//     current one.
//   - A player sees a group that holds a player's character with the members
//     they see (never a hidden one), and the first of them who acts is the
//     current one. When every member who acts is hidden, it is the master's
//     turn ("Vez do mestre", or "Falta o mestre").
//   - A group of NPCs alone is the master's: the player gets the members they
//     see so the app can say "Vez dos Goblins", but no member is made the
//     current one when there are two or more, and no flags.
func (d *encounterData) turnFor(v combatViewer) turnView {
	if d.enc.Status != statusActive {
		return turnView{}
	}
	members := turnMembers(d.cs)
	acting := func(cs []playdb.Combatant) (playdb.Combatant, bool) {
		// A living member is the current one; a defeated one only when it is the
		// last that has not ended its part.
		found, ok := playdb.Combatant{}, false
		for _, c := range cs {
			if c.TurnState != turnActing {
				continue
			}
			if !c.Defeated {
				return c, true
			}
			if !ok {
				found, ok = c, true
			}
		}
		return found, ok
	}
	if _, someone := acting(members); !someone {
		return turnView{}
	}
	ids := func(cs []playdb.Combatant) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.ID)
		}
		return out
	}
	if v.master {
		first, _ := acting(members)
		return turnView{currentID: first.ID, groupIDs: ids(members), flags: true}
	}
	var seen []playdb.Combatant
	for _, c := range members {
		if v.sees(c) {
			seen = append(seen, c)
		}
	}
	hasPlayer := slices.ContainsFunc(members, inParty) // a player's character or its creature: a player plays it
	first, visibleActs := acting(seen)
	if !visibleActs {
		return turnView{masterTurn: true, groupIDs: ids(seen), flags: hasPlayer} // only hidden members act
	}
	if !hasPlayer && len(seen) > 1 {
		return turnView{groupIDs: ids(seen)} // NPCs alone: named by the group, none picked
	}
	return turnView{currentID: first.ID, groupIDs: ids(seen), flags: hasPlayer}
}

// npcOnlyGroups are the groups of two or more NPCs, adjacent in the order with
// the same total, that a player may see: without hidden or defeated members,
// as lists of IDs. Only for the player's copy, to name a group by its plural.
func (d *encounterData) npcOnlyGroups(v combatViewer) []*playv1.NpcGroup {
	if v.master || d.enc.Status != statusActive {
		return nil
	}
	// The runs come from the whole order, so a hidden or defeated combatant in
	// between never makes two groups one; only then each run is filtered for
	// the viewer, and kept when it has two or more visible NPCs and no player.
	var out []*playv1.NpcGroup
	for _, g := range groupRuns(d.cs) {
		if slices.ContainsFunc(g, func(c playdb.Combatant) bool { return c.Kind != kindNPC }) {
			continue
		}
		var ids []string
		for _, c := range g {
			if !c.Defeated && v.sees(c) {
				ids = append(ids, c.ID)
			}
		}
		if len(ids) >= 2 {
			out = append(out, &playv1.NpcGroup{CombatantIds: ids})
		}
	}
	return out
}

// view builds the Encounter the viewer sees. vitals are the player
// characters' vitals by character ID; a player only learns from them that a
// character is down, and the master gets the hit points.
func (d *encounterData) view(v combatViewer, vitals map[string]*playv1.CharacterVitals, armorClass map[string]int32, npcs map[string]masterNPC, names func(key string) string) *playv1.Encounter {
	e := d.enc
	out := &playv1.Encounter{
		Id:          e.ID,
		Name:        e.Name,
		Status:      statusToProto[e.Status],
		Round:       e.Round,
		MapId:       deref(e.MapID),
		GridColumns: e.GridColumns,
		GridRows:    e.GridRows,
		Mode:        modeProto(e.Mode),
		Revision:    e.Revision,
		StartedAt:   timestampOrNil(e.StartedAt),
		EndedAt:     timestampOrNil(e.EndedAt),
	}
	if v.master {
		out.MapPointId = deref(e.MapPointID)
	}
	turn := d.turnFor(v)
	out.CurrentCombatantId, out.MasterTurn, out.TurnGroupIds = turn.currentID, turn.masterTurn, turn.groupIDs
	out.NpcOnlyGroups = d.npcOnlyGroups(v)
	ties := unresolvedTies(d.cs)
	// Players in the same joint turn read each other's economy: they act
	// together and say what each still has. A player outside the group does not.
	shared := !v.master && slices.ContainsFunc(d.cs, func(c playdb.Combatant) bool { return v.owns(c) && inTurn(e, c) })
	for _, c := range d.cs {
		if v.sees(c) {
			var vit *playv1.CharacterVitals
			if c.Kind == kindPlayer { // a creature's character_id is its owner's: never the owner's vitals
				vit = vitals[c.CharacterID]
			}
			p := combatantToProto(c, v, ties[c.ID], vit, armorClass[sheetKey(c)], npcs[sheetKey(c)].portrait, actsNow(e, c), names)
			if v.master { // the creature of a monster: the master's alone (RN-20)
				p.BestiaryCreatureKey, p.ChallengeRating = npcs[sheetKey(c)].monsterKey, npcs[sheetKey(c)].challengeRating
			}
			if shared && inParty(c) && !v.owns(c) && inTurn(e, c) {
				shareEconomy(p, c)
			}
			p.TurnPartEnded = turn.flags && e.Status == statusActive && c.TurnState == turnEnded
			out.Combatants = append(out.Combatants, p)
		}
	}
	return out
}

// combatantToProto builds the Combatant the viewer sees. The caller checked
// that the viewer sees it. armorClass is its sheet's, and portrait the URL of
// its sheet's portrait, for the master's copy only (0 and "" when unknown).
// onTurn says it acts now: it is in the group on turn and its part has not
// ended, in a running combat.
func combatantToProto(c playdb.Combatant, v combatViewer, tieUnresolved bool, vitals *playv1.CharacterVitals, armorClass int32, portrait string, onTurn bool, names func(key string) string) *playv1.Combatant {
	controls := v.owns(c)                    // the caller's player plays it: their character or its creature
	mine := controls && c.Kind == kindPlayer // only their own character is "mine"; the web finds its combatant with it
	detail := v.master || controls           // the numbers of the turn: the master's and the player's
	out := &playv1.Combatant{
		Id:                 c.ID,
		Label:              c.Label,
		Kind:               kindToProto[c.Kind],
		Mine:               mine,
		ControlledByMe:     controls,
		Placed:             placed(c),
		State:              stateOf(c),
		Defeated:           c.Defeated,
		DeathSuccesses:     c.DeathSuccesses,
		DeathFailures:      c.DeathFailures,
		Conditions:         c.Conditions,
		ConcentrationSpell: deref(c.ConcentrationSpell),
		Side:               sideProto(c.Side),
		Size:               sizeProto(c.Size),
		CoverMark:          coverDegreeProto(markKeyOf(c)),
	}
	if v.deathHiddenFrom(c) {
		// The table keeps the death saves to the owner and the master (RN-24): the
		// counts are not sent. The state word below says "Caído", or the result
		// ("Estável", "Morto").
		out.DeathSuccesses, out.DeathFailures = 0, 0
	}
	for _, key := range c.Conditions {
		out.ConditionNamesPt = append(out.ConditionNamesPt, names(key))
	}
	if out.ConcentrationSpell != "" {
		out.ConcentrationSpellNamePt = names(out.ConcentrationSpell)
	}
	if placed(c) {
		out.Col, out.Row = *c.GridCol, *c.GridRow
	}
	switch {
	case isCreature(c):
		// A creature stands for no character of its own: its owner is public (the
		// party knows whose it is), the rest is its owner's and the master's.
		out.OwnerCharacterId = c.CharacterID
		out.MonsterKey, out.MonsterNamePt = deref(c.MonsterKey), names(deref(c.MonsterKey))
		if detail {
			out.CreatureId, out.CreatureAttack, out.SummonGroupId = deref(c.CreatureID), creatureAttackToProto[deref(c.SummonAttack)], deref(c.SummonGroupID)
		}
	case c.Kind == kindPlayer || v.master:
		out.CharacterId = c.CharacterID // an NPC's character is the master's secret
	}
	// A player's roll is public among the players, and so is a creature's: it
	// is on the party's side. An NPC's never reaches a player (RN-20).
	if c.Initiative != nil && (v.master || inParty(c)) {
		out.Initiative = c.Initiative
	}
	if detail {
		out.InitiativeBonus = new(c.InitiativeBonus)
		out.InitiativeFace = c.InitiativeFace
		shareEconomy(out, c)
		out.ArmorClassBonus = c.AcBonus
		out.DeathSaveDue = onTurn && deathSaveDue(c, vitals)
	}
	if w := vitals.GetWildShape(); w != nil && c.Kind == kindPlayer {
		// The party sees the wolf; the beast's hit points are numbers for the master
		// and its own player, like the character's (RN-20).
		out.WildShapeBeastKey, out.WildShapeBeastNamePt = w.GetBeastKey(), w.GetBeastNamePt()
		if detail {
			out.WildShapeHitPointsCurrent, out.WildShapeHitPointsMax = new(w.GetHitPointsCurrent()), new(w.GetHitPointsMax())
		}
	}
	if fs := vitals.GetFamiliarSight(); fs != nil && c.Kind == kindPlayer && detail {
		out.FamiliarSightCreatureId = fs.GetCreatureId()
	}
	if controls && isCreature(c) {
		// A creature's hit points are numbers to its owner's player and the master
		// (below); everyone else gets the state word.
		out.HitPointsCurrent, out.HitPointsMax, out.HitPointsTemporary = c.HpCurrent, c.HpMax, c.HpTemp
	}
	if v.master {
		out.Hidden = c.Hidden
		out.TieUnresolved = tieUnresolved
		if armorClass > 0 {
			out.ArmorClass = new(armorClass)
		}
		out.HitPointsCurrent, out.HitPointsMax, out.HitPointsTemporary = c.HpCurrent, c.HpMax, c.HpTemp
		out.XpValue = c.XpValue // an NPC's, the master's alone (RN-20); 0 for a player's character
		out.PortraitUrl = portrait
		if vitals != nil {
			out.HitPointsCurrent = new(vitals.GetHitPointsCurrent())
			out.HitPointsMax = new(vitals.GetHitPointsMax())
			out.HitPointsTemporary = new(vitals.GetHitPointsTemporary())
		}
	}
	// A player's character at 0 hit points is down ("Caído"): everyone who
	// sees it gets the word, never the numbers (those are the master's, above).
	// With three death save successes it is "Estável"; with three failures it is
	// "Morrendo" for the master, and still "Caído" for the players, who never
	// hear it died before the master confirms (RN-03). A confirmed death is
	// "Morto".
	switch {
	case c.Kind == kindPlayer && c.Defeated:
		out.State = playv1.CombatantState_COMBATANT_STATE_DEAD
	case c.Kind == kindPlayer && isDownIn(vitals):
		out.State = playv1.CombatantState_COMBATANT_STATE_DOWN
		switch {
		case c.DeathSuccesses >= 3:
			out.State = playv1.CombatantState_COMBATANT_STATE_STABLE
		case c.DeathFailures >= 3 && v.master:
			out.State = playv1.CombatantState_COMBATANT_STATE_DYING
		}
	}
	return out
}

// shareEconomy gives a player's combatant's turn economy to another player who
// is in the same joint turn (the rest of the detail stays the owner's).
func shareEconomy(out *playv1.Combatant, c playdb.Combatant) {
	// The movement is kept in tenths of a foot; the `_ft` fields are those rounded
	// down, for the web that is already shipped (RN-21).
	out.SpeedFt = c.SpeedFt
	out.SpeedFlyFt = c.SpeedFlyFt
	out.SpeedDft = clamp32(speedDFt(c), 0, math.MaxInt32)
	out.MovementUsedFt = c.MovementUsedDft / 10
	out.MovementUsedDft = c.MovementUsedDft
	out.MovementLeftFt = clamp32(movementLeftFt(c), 0, 1200) // twice the largest speed
	out.MovementLeftDft = clamp32(movementLeftDFt(c), 0, math.MaxInt32)
	out.Dashed, out.ActionUsed, out.BonusActionUsed, out.ReactionUsed = c.Dashed, c.ActionUsed, c.BonusActionUsed, c.ReactionUsed
	out.Disengaged = c.Disengaged
}

// markKeyOf is the cover the master marked, as an event keeps it ("" for none).
func markKeyOf(c playdb.Combatant) string { return coverKeys[coverToGrid[c.CoverMark]] }

// isDownIn says whether the vitals are of a character at 0 hit points.
func isDownIn(v *playv1.CharacterVitals) bool {
	return v.GetHitPointsMax() > 0 && v.GetHitPointsCurrent() == 0
}

// deathSaveDue says whether a player's character, on turn, owes a death save:
// it is down, still making them (not stable, not dying) and has not rolled this
// turn.
func deathSaveDue(c playdb.Combatant, vitals *playv1.CharacterVitals) bool {
	return c.Kind == kindPlayer && !c.Defeated && isDownIn(vitals) &&
		c.DeathSuccesses < 3 && c.DeathFailures < 3 && !c.DeathSaveRolled
}

// The database's statuses and kinds, and the API's.
var (
	statusToProto = map[string]playv1.EncounterStatus{
		statusSetup:  playv1.EncounterStatus_ENCOUNTER_STATUS_SETUP,
		statusActive: playv1.EncounterStatus_ENCOUNTER_STATUS_ACTIVE,
		statusEnded:  playv1.EncounterStatus_ENCOUNTER_STATUS_ENDED,
	}
	kindToProto = map[string]playv1.CombatantKind{
		kindPlayer:   playv1.CombatantKind_COMBATANT_KIND_PLAYER,
		kindNPC:      playv1.CombatantKind_COMBATANT_KIND_NPC,
		kindCreature: playv1.CombatantKind_COMBATANT_KIND_CREATURE,
	}
	// creatureAttackToProto is what the spell lets a creature do (rules.SummonAttack*).
	creatureAttackToProto = map[string]playv1.CreatureAttack{
		"none":     playv1.CreatureAttack_CREATURE_ATTACK_NONE,
		"reaction": playv1.CreatureAttack_CREATURE_ATTACK_REACTION,
		"full":     playv1.CreatureAttack_CREATURE_ATTACK_FULL,
	}
)

func timestampOrNil(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

// viewFor builds the combat for a caller. It reads the player characters'
// vitals: the master's copy carries their hit points, and every copy says
// which of them are down.
func (s *Service) viewFor(ctx context.Context, m authz.Membership, d *encounterData) (*playv1.Encounter, error) {
	v, err := s.viewerFor(ctx, m, d.enc, d.cs)
	if err != nil {
		return nil, s.dbError(ctx, "work out what the player sees", err)
	}
	var byCharacter map[string]*playv1.CharacterVitals
	if slices.ContainsFunc(d.cs, func(c playdb.Combatant) bool { return c.Kind == kindPlayer }) {
		all, err := s.vitals.ListVitals(ctx, m.CampaignID)
		if err != nil {
			return nil, s.dbError(ctx, "list vitals", err)
		}
		byCharacter = make(map[string]*playv1.CharacterVitals, len(all))
		for _, vit := range all {
			byCharacter[vit.GetCharacterId()] = vit
		}
	}
	names := s.namesFor(ctx, m.CampaignID) // the content is read once for the whole view
	out := d.view(v, byCharacter, s.armorClasses(ctx, m, d), s.npcDetails(ctx, m, d), names)
	if v.sight != nil { // a fog map: the revision is what this player could see happen
		if out.Revision, err = s.visibleRevision(ctx, d.enc.ID, v.userID); err != nil {
			return nil, s.dbError(ctx, "count what the player could see", err)
		}
	}
	prompts, err := s.reactionPrompts(ctx, m, d, v, names)
	if err != nil {
		return nil, err
	}
	out.ReactionPrompts = prompts
	if out.OpportunityOffers, err = s.opportunityOffers(ctx, m, d, v); err != nil {
		return nil, err
	}
	if err := s.creatureView(ctx, m, d, out); err != nil {
		return nil, err
	}
	return out, nil
}

// armorClasses reads the armor class of each character in the combat, for
// the master's copy only (RN-20): a player never gets one. Copies of an NPC
// share a character, so each sheet is read once. A sheet that can't be read
// (the character left the campaign) simply has no armor class on screen.
func (s *Service) armorClasses(ctx context.Context, m authz.Membership, d *encounterData) map[string]int32 {
	if m.Role != authz.RoleMaster {
		return nil
	}
	out := make(map[string]int32, len(d.cs))
	for _, c := range d.cs {
		if _, done := out[sheetKey(c)]; done {
			continue
		}
		sheet, err := s.sheetOf(ctx, nil, m.CampaignID, c)
		if err != nil {
			continue
		}
		out[sheetKey(c)] = clamp32(sheet.ArmorClass, 0, math.MaxInt32)
	}
	return out
}

// masterNPC is what the master's copy shows of an NPC besides its numbers: its
// portrait and, for a monster of the bestiary, the creature and its ND.
type masterNPC struct {
	portrait, monsterKey, challengeRating string
}

// npcDetails reads the portrait URL of each NPC in the combat and, for a monster, its
// creature and ND, for the master's card only (MR-031, RN-29): a player sees a
// portrait only on the stage, and only the name and the image there, and never the
// creature. A sheet that can't be read gives none: the app draws the initials.
func (s *Service) npcDetails(ctx context.Context, m authz.Membership, d *encounterData) map[string]masterNPC {
	if m.Role != authz.RoleMaster {
		return nil
	}
	var ids []string
	for _, c := range d.cs {
		if c.Kind == kindNPC && !slices.Contains(ids, c.CharacterID) {
			ids = append(ids, c.CharacterID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	chars, err := s.roster.CombatCharacters(ctx, nil, m.CampaignID, ids)
	if err != nil {
		return nil
	}
	out := make(map[string]masterNPC, len(chars))
	for _, c := range chars {
		n := masterNPC{monsterKey: c.MonsterKey, challengeRating: c.ChallengeRating}
		if c.PortraitImageID != "" {
			n.portrait = imagesPath + c.PortraitImageID
		}
		out[c.ID] = n
	}
	return out
}

// GetEncounter implements playv1connect.CombatServiceHandler.
func (s *Service) GetEncounter(
	ctx context.Context,
	req *connect.Request[playv1.GetEncounterRequest],
) (*connect.Response[playv1.GetEncounterResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	// The session, the latest encounter and its combatants are one snapshot
	// (audit D-03): apart, a write between them gave a view of two states. What
	// the view adds on top (vitals, sheets, the fog) is read after.
	var d *encounterData
	err = db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		session, err := openSessionWith(ctx, q, m.CampaignID)
		if err != nil {
			return err
		}
		enc, err := q.GetLatestEncounter(ctx, session.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			d = nil // no encounter yet
			return nil
		}
		if err != nil {
			return fmt.Errorf("find the encounter: %w", err)
		}
		d, err = loadEncounter(ctx, q, enc)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "read the encounter", err)
	}
	if d == nil {
		return connect.NewResponse(&playv1.GetEncounterResponse{}), nil
	}
	out, err := s.viewFor(ctx, m, d)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.GetEncounterResponse{Encounter: out}), nil
}

// The live events of a combat (play.proto, WatchGameSessionResponse). They
// go out after the commit. encounter_changed is a hint without content, so
// everyone gets it; turn_changed and combatant_moved carry names of squares
// and turns, so each audience gets only what it may see.

// publishEncounterChanged tells everyone to read the combat again.
//
// On a map with the fog of war the revision is the master's alone: it goes up for
// everything that happens, so a player who got it could count the moves in the dark.
// The players' hint says 0, "read it again" (and so does the failure to tell).
func (s *Service) publishEncounterChanged(ctx context.Context, campaignID string, e playdb.Encounter) {
	msg := encounterChangedMessage(e)
	f, err := s.fogSightOf(ctx, campaignID, e)
	if err != nil || f != nil {
		s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Master: true}, Message: msg})
		blind := playdb.Encounter{ID: e.ID, Mode: e.Mode}
		s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Players: true}, Message: encounterChangedMessage(blind)})
		return
	}
	s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Everyone: true}, Message: msg})
}

// publishEncounterChangedFor is publishEncounterChanged for a change made only to the
// combatants ids: when all of them are hidden the players' view did not change, so the hint
// goes to the master alone and the players cannot count the master's edits of what they
// do not see.
func (s *Service) publishEncounterChangedFor(ctx context.Context, campaignID string, d *encounterData, ids ...string) {
	if allHidden(d.cs, ids) {
		s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Master: true}, Message: encounterChangedMessage(d.enc)})
		return
	}
	s.publishEncounterChanged(ctx, campaignID, d.enc)
}

// allHidden says whether every id is a combatant of cs that is hidden now.
func allHidden(cs []playdb.Combatant, ids []string) bool {
	if len(ids) == 0 {
		return false
	}
	return !slices.ContainsFunc(ids, func(id string) bool {
		i := slices.IndexFunc(cs, func(c playdb.Combatant) bool { return c.ID == id })
		return i < 0 || !cs[i].Hidden
	})
}

// changedFor is the usual publish for a change made only to the combatants ids.
func (s *Service) changedFor(campaignID string, ids ...string) func(ctx context.Context, d *encounterData) {
	return func(ctx context.Context, d *encounterData) { s.publishEncounterChangedFor(ctx, campaignID, d, ids...) }
}

// publishTurnChanged tells who is on turn: the master the real combatant,
// the players what they may see (a hidden one's turn is "the master's"; on a
// map with the fog of war, so is the turn of an NPC the player does not see).
func (s *Service) publishTurnChanged(ctx context.Context, campaignID string, d *encounterData) {
	s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Master: true}, Message: turnChangedMessage(d.enc, d.turnFor(combatViewer{master: true}))})
	s.publishTurnChangedToPlayers(ctx, campaignID, d)
}

// publishCombatantMoved tells the master always, and the players only when the
// combatant is not hidden from them (on a map with the fog of war: when they see
// its square, see publishMovedToPlayers).
func (s *Service) publishCombatantMoved(ctx context.Context, campaignID string, e playdb.Encounter, c playdb.Combatant, from *grid.Square) {
	if !placed(c) {
		return
	}
	s.hub.Publish(campaignID, live.Event{Audience: live.Audience{Master: true}, Message: combatantMovedMessage(e, c)})
	if !c.Hidden {
		s.publishMovedToPlayers(ctx, campaignID, e, c, from)
	}
}
