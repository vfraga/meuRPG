package characters

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// Vitals (RN-02): a player character's current and temporary hit points,
// spell slots used, pact magic slots used and hit dice used, which last
// from one game session to the next (table character_vitals).
//
// The maximums are never stored: they come from rules.Derive, like every
// number on the sheet, and the stored values are clamped to them on every
// read. A character without a row is fresh: full hit points, nothing used.
//
// Package play serves the vitals (PlayService: GetLiveSession, the live
// stream, AdjustCharacterVitals) through its VitalsKeeper interface, which
// this Service implements with ListVitals, GetVitals and AdjustVitals. The
// messages are play's (playv1.CharacterVitals): play declares the interface
// and what goes through it, as with SheetLocker, and this package only
// fills them in. So this package imports play's generated API types
// (gen/meurpg/play/v1), never package play itself; cmd/api connects the
// two.

// MaxTemporaryHitPoints is the most temporary hit points a character can
// have. The rules set no maximum; this one only keeps typos out.
const MaxTemporaryHitPoints = 999

// maxSpellLevel is the highest spell level, the length of
// character_vitals.spell_slots_used.
const maxSpellLevel = 9

// vitalsRow is a character with its stored vitals: a row of ListVitals or
// GetVitals, which have the same columns. The vitals columns are nil when
// the character has no character_vitals row.
type vitalsRow = charactersdb.ListVitalsRow

// vitalsMax are the maximums, derived from the sheet.
type vitalsMax struct {
	hitPoints int
	// slots[k] is the number of slots of spell level k+1.
	slots        [maxSpellLevel]int
	pactLevel    int
	pactSlots    int
	hitDice      []rules.HitDice
	hitDiceTotal int
	// wizardLevel is the WIZARD level alone (Arcane Recovery counts no other class).
	wizardLevel int
	// songSides is the die of the character's Song of Rest (0 without the feature).
	songSides int
	// conMod is the Constitution modifier: what a spent hit die adds to its roll.
	conMod int
	// resources are the class and race resources the sheet has at its level.
	resources []rules.Resource
	// beast is the Wild Shape form the character is in (MR-037): its content key,
	// Portuguese name and the stat block's hit points. Empty key: its own shape.
	beast       string
	beastNamePT string
	beastMax    int
}

// maxima derives the maximums from a stored sheet. A player character
// always has a full sheet; anything else has no hit points, slots or dice.
func maxima(content *rules.Content, characterID string, doc []byte, beast *string) (vitalsMax, error) {
	sheet, err := loadSheet(characterID, doc)
	if err != nil {
		return vitalsMax{}, err
	}
	full := sheet.GetFull()
	if full == nil {
		return vitalsMax{}, nil
	}
	d := rules.Derive(buildOf(full), content)
	m := vitalsMax{hitPoints: max(d.HitPointsMax, 0), hitDice: d.HitDice, resources: d.Resources, songSides: d.SongOfRestSides}
	if b, ok := content.MonsterDerived(deref(beast)); ok && beast != nil {
		m.beast, m.beastNamePT, m.beastMax = *beast, content.NamePT(*beast), max(b.HitPointsMax, 1)
	}
	for i, n := range d.SpellSlots {
		if i < maxSpellLevel {
			m.slots[i] = max(n, 0)
		}
	}
	if d.PactMagic != nil {
		m.pactLevel, m.pactSlots = d.PactMagic.SlotLevel, max(d.PactMagic.Slots, 0)
	}
	for _, hd := range d.HitDice {
		m.hitDiceTotal += hd.Count
	}
	for _, c := range full.GetClasses() {
		if c.GetClassKey() == "class:wizard" {
			m.wizardLevel += int(c.GetLevel())
		}
	}
	for _, a := range d.Abilities {
		if a.Ability == rules.CON {
			m.conMod = a.Modifier
		}
	}
	return m, nil
}

// liveFamiliar forgets the sight of a familiar that is not with the character any
// more (dismissed, or deleted): the player looks through its own eyes again. A sight
// that still holds the conditions it gave the combatant stays: play ends it with the
// owner's next turn or the end of the combat and takes those conditions back, which it
// can only do while it sees the sight. It runs only for a sight that is on, so the
// usual read asks nothing more.
func (s *Service) liveFamiliar(ctx context.Context, q *charactersdb.Queries, campaignID string, row *vitalsRow) error {
	if row.FamiliarSightCreatureID == nil || len(row.FamiliarSightConditions) > 0 {
		return nil
	}
	found, err := q.ListCreaturesByIDs(ctx, charactersdb.ListCreaturesByIDsParams{CampaignID: campaignID, Ids: []string{*row.FamiliarSightCreatureID}})
	if err != nil {
		return err
	}
	if len(found) == 0 || found[0].CharacterCreature.DismissedAt != nil {
		row.FamiliarSightCreatureID = nil
	}
	return nil
}

// vitalsToProto merges the stored values with the maximums, clamping each
// value to its maximum: a sheet that lost a level, or a class, never shows
// more than it has now.
func vitalsToProto(row vitalsRow, m vitalsMax) *playv1.CharacterVitals {
	bonus := derefInt(row.HitPointsMaxBonus) // Aid's, on top of the sheet's maximum
	// From level 4 of exhaustion the maximum is halved (SRD, Conditions).
	level := min(max(derefInt(row.ExhaustionLevel), 0), combat.MaxExhaustion)
	maxHP := i32(combat.ExhaustedMaxHP(m.hitPoints+int(bonus), int(level)))
	v := &playv1.CharacterVitals{
		CharacterId:             row.ID,
		Name:                    row.Name,
		PlayerUserId:            deref(row.PlayerUserID),
		HitPointsCurrent:        maxHP, // fresh: full hit points
		HitPointsMax:            maxHP,
		ExhaustionLevel:         level,
		ArmorClassBase:          derefInt(row.ArmorClassBase),
		ArcaneRecoveryAllowance: i32(rules.ArcaneRecoveryAllowance(m.wizardLevel)),
		HitPointsMaxBonus:       bonus,
		HitPointsTemporary:      derefInt(row.HitPointsTemporary),
		HitDiceTotal:            i32(m.hitDiceTotal),
		Revision:                derefInt(row.Revision),
		UpdatedAt:               timestamp(row.UpdatedAt),
	}
	if row.HitPointsCurrent != nil {
		v.HitPointsCurrent = min(*row.HitPointsCurrent, v.HitPointsMax)
	}
	// The hit dice spent, by size: the sum is the count the older clients read.
	spent := hitDiceUsedOf(row, m)
	v.HitDiceUsed = i32(spent.Total())
	v.HitDiceUsedByDie = hitDiceToProto(spent)
	for k, sheetTotal := range m.slots {
		// The slots Flexible Casting created add to the sheet's.
		var created int32
		if k < len(row.SpellSlotsCreated) {
			created = max(row.SpellSlotsCreated[k], 0)
		}
		total := sheetTotal + int(created)
		if total == 0 {
			continue
		}
		var used int32
		if k < len(row.SpellSlotsUsed) {
			used = min(row.SpellSlotsUsed[k], i32(total))
		}
		v.SpellSlots = append(v.SpellSlots, &playv1.SpellSlotUsage{Level: i32(k + 1), Total: i32(total), Used: used, Created: created})
	}
	if m.pactSlots > 0 {
		v.PactSlots = &playv1.PactSlotUsage{
			SlotLevel: i32(m.pactLevel),
			Total:     i32(m.pactSlots),
			Used:      min(derefInt(row.PactSlotsUsed), i32(m.pactSlots)),
		}
	}
	for _, hd := range m.hitDice {
		v.HitDice = append(v.HitDice, &rulesv1.HitDice{Faces: i32(hd.Die), Count: i32(hd.Count)})
	}
	// The stored uses are a JSON object {key: n}; a value that does not read
	// counts as nothing spent, and each is cut to the sheet's total.
	var used map[string]int32
	_ = json.Unmarshal(row.ResourcesUsed, &used)
	for _, r := range m.resources {
		v.Resources = append(v.Resources, &playv1.ResourceUsage{
			Key: r.Key, NamePt: r.NamePT, Total: i32(r.Max), Used: min(max(used[r.Key], 0), i32(r.Max)), Recharge: rechargeToProto[r.Recharge],
		})
	}
	if row.WildShapeBeast != nil && m.beast == *row.WildShapeBeast && row.WildShapeHp != nil {
		// The beast's pool is cut to the stat block's maximum, as the character's is.
		v.WildShape = &playv1.WildShapeState{
			BeastKey: m.beast, BeastNamePt: m.beastNamePT, HitPointsCurrent: min(max(*row.WildShapeHp, 1), i32(m.beastMax)), HitPointsMax: i32(m.beastMax),
		}
	}
	if row.FamiliarSightCreatureID != nil {
		v.FamiliarSight = &playv1.FamiliarSightState{
			CreatureId: *row.FamiliarSightCreatureID, InCombat: derefBool(row.FamiliarSightInCombat), ConditionsGiven: row.FamiliarSightConditions,
		}
	}
	return v
}

// ListVitals returns the vitals of the campaign's living, active player
// characters, oldest first (RN-02): not NPCs, not the dead, not a
// character waiting for approval. Package play calls it after its own
// authorization check, and chooses which of them the caller may see.
func (s *Service) ListVitals(ctx context.Context, campaignID string) ([]*playv1.CharacterVitals, error) {
	rows, err := s.queries.ListVitals(ctx, campaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list vitals", err)
	}
	content, err := s.contentFor(ctx, nil, campaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list vitals", err)
	}
	out := make([]*playv1.CharacterVitals, 0, len(rows))
	for _, row := range rows {
		m, err := maxima(content, row.ID, row.Sheet, row.WildShapeBeast)
		if err != nil {
			return nil, s.dbError(ctx, "list vitals", err)
		}
		if err := s.liveFamiliar(ctx, s.queries, campaignID, &row); err != nil {
			return nil, s.dbError(ctx, "list vitals", err)
		}
		out = append(out, vitalsToProto(row, m))
	}
	return out, nil
}

// GetVitals returns one living, active player character's vitals, or a
// `not_found` Connect error when characterID is not one in the campaign.
func (s *Service) GetVitals(ctx context.Context, campaignID, characterID string) (*playv1.CharacterVitals, error) {
	return s.getVitals(ctx, nil, campaignID, characterID, false)
}

// GetVitalsTx is GetVitals inside tx, so a change that computes from the
// vitals and writes them back (the master applying damage) reads what its own
// transaction will overwrite, never a stale copy.
func (s *Service) GetVitalsTx(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (*playv1.CharacterVitals, error) {
	return s.getVitals(ctx, tx, campaignID, characterID, false)
}

// getVitals reads the vitals in tx (nil: the pool). With withDead, the vitals of a
// character that died answer too: only the reads that serve a dead character's
// player ask for it; anything that lets a character act or heals it does not.
func (s *Service) getVitals(ctx context.Context, tx pgx.Tx, campaignID, characterID string, withDead bool) (*playv1.CharacterVitals, error) {
	q := s.queriesIn(tx)
	id, ok := parseUUID(characterID)
	if !ok {
		return nil, errCharacterNotFound()
	}
	var row charactersdb.GetVitalsRow
	var err error
	if withDead {
		var dead charactersdb.GetVitalsWithDeadRow
		dead, err = q.GetVitalsWithDead(ctx, charactersdb.GetVitalsWithDeadParams{CampaignID: campaignID, ID: id})
		row = charactersdb.GetVitalsRow(dead)
	} else {
		row, err = q.GetVitals(ctx, charactersdb.GetVitalsParams{CampaignID: campaignID, ID: id})
	}
	if err != nil {
		return nil, s.dbError(ctx, "get vitals", err) // no row: not_found
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return nil, s.dbError(ctx, "get vitals", err)
	}
	m, err := maxima(content, row.ID, row.Sheet, row.WildShapeBeast)
	if err != nil {
		return nil, s.dbError(ctx, "get vitals", err)
	}
	current := vitalsRow(row)
	if err := s.liveFamiliar(ctx, q, campaignID, &current); err != nil {
		return nil, s.dbError(ctx, "get vitals", err)
	}
	return vitalsToProto(current, m), nil
}

// AdjustVitals applies the master's correction (RN-02) inside tx, and
// returns the vitals before and after it. Each value set in req replaces
// the current one; the rest stay. It reads only the vitals fields of req,
// and its errors are Connect errors to return as they are:
//   - `not_found`: characterID is not a living, active player character of
//     the campaign;
//   - `invalid_argument`: nothing to change, or a value outside 0 to its
//     maximum, naming the request's field.
//
// Package play calls it from AdjustCharacterVitals, after checking that
// the caller is the campaign's master and that a session is open, in the
// transaction that also writes the session event. It takes no caller on
// purpose, like LockSheets.
func (s *Service) AdjustVitals(ctx context.Context, tx pgx.Tx, campaignID, characterID string, req *playv1.AdjustCharacterVitalsRequest) (before, after *playv1.CharacterVitals, err error) {
	r, _, err := s.vitalsOfRow(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, nil, err
	}
	before = r.view
	after = proto.CloneOf(before)
	if err := applyVitalsChange(after, req); err != nil {
		return nil, nil, invalidArgument(err)
	}
	params, err := correctedVitals(r.row, req, after)
	if err != nil {
		return nil, nil, err
	}
	q := s.queries.WithTx(tx)
	params.Now = s.now()
	saved, err := q.UpsertVitals(ctx, params)
	if err != nil {
		return nil, nil, wrap("save vitals", err)
	}
	if err := s.saveWildShapeChange(ctx, q, r.row.ID, req, after); err != nil {
		return nil, nil, err
	}
	after.Revision = saved.Revision
	after.UpdatedAt = timestamppb.New(saved.UpdatedAt)
	return before, after, nil
}

// correctedVitals is the row a correction stores, all but the time. Usage is stored as
// it was and changed only where req sets it: the view is cut to today's sheet, and
// writing that back would erase the usage of slots and resources the sheet has lost for
// now.
func correctedVitals(row vitalsRow, req *playv1.AdjustCharacterVitalsRequest, after *playv1.CharacterVitals) (charactersdb.UpsertVitalsParams, error) {
	used := make([]int32, maxSpellLevel)
	copy(used, row.SpellSlotsUsed)
	for _, change := range req.GetSpellSlotsUsed() {
		used[change.GetLevel()-1] = change.GetUsed()
	}
	pactUsed := derefInt(row.PactSlotsUsed)
	if req.PactSlotsUsed != nil {
		pactUsed = after.GetPactSlots().GetUsed()
	}
	// The hit dice are kept by size. A correction that leaves them alone writes what
	// was read (a row from before the sizes were kept gets them now).
	hitDiceJSON, hitDiceUsed, err := encodeHitDiceUsed(hitDiceFromProto(after.GetHitDiceUsedByDie()))
	if err != nil {
		return charactersdb.UpsertVitalsParams{}, wrap("encode the hit dice", err)
	}
	usedResources := storedResourcesUsed(row.ResourcesUsed)
	for _, change := range req.GetResourcesUsed() {
		setResourceUse(usedResources, change.GetKey(), change.GetUsed())
	}
	resourcesJSON, err := json.Marshal(usedResources)
	if err != nil {
		return charactersdb.UpsertVitalsParams{}, wrap("encode the resources", err)
	}
	// A character whose hit points were never set, and this change leaves alone,
	// keeps them unset: full, whatever the maximum becomes.
	hitPoints := &after.HitPointsCurrent
	if req.HitPointsCurrent == nil && row.HitPointsCurrent == nil {
		hitPoints = nil
	}
	return charactersdb.UpsertVitalsParams{
		CharacterID:        row.ID,
		HitPointsCurrent:   hitPoints,
		HitPointsTemporary: after.GetHitPointsTemporary(),
		SpellSlotsUsed:     used,
		PactSlotsUsed:      pactUsed,
		HitDiceUsed:        hitDiceUsed,
		HitDiceUsedByDie:   hitDiceJSON,
		SpellSlotsCreated:  slotsCreatedOf(row),
		ResourcesUsed:      resourcesJSON,
	}, nil
}

// storedResourcesUsed reads the stored uses of the resources, a JSON object {key: n}; a
// value that does not read counts as nothing spent. The map is never nil.
func storedResourcesUsed(raw []byte) map[string]int32 {
	used := map[string]int32{}
	_ = json.Unmarshal(raw, &used)
	if used == nil {
		used = map[string]int32{}
	}
	return used
}

// setResourceUse puts a resource's uses in the stored map: none is no entry.
func setResourceUse(used map[string]int32, key string, n int32) {
	if n > 0 {
		used[key] = n
	} else {
		delete(used, key)
	}
}

// saveWildShapeChange stores the beast's pool when the correction set it. It is a row of
// its own (character_wild_shapes): 0 ended the form (MR-037). UpsertVitals bumped the
// revision already.
func (s *Service) saveWildShapeChange(ctx context.Context, q *charactersdb.Queries, characterID string, req *playv1.AdjustCharacterVitalsRequest, after *playv1.CharacterVitals) error {
	if req.WildShapeHitPointsCurrent == nil {
		return nil
	}
	var err error
	if w := after.GetWildShape(); w != nil {
		err = q.SetWildShape(ctx, charactersdb.SetWildShapeParams{CharacterID: characterID, Beast: w.GetBeastKey(), Hp: w.GetHitPointsCurrent(), Now: s.now()})
	} else {
		err = q.ClearWildShape(ctx, characterID)
	}
	if err != nil {
		return wrap("save the wild shape", err)
	}
	return nil
}

// applyVitalsChange sets on v the values req sets, checking each against
// its maximum. v already has the maximums. Errors are fieldErrors, with the
// request's field names. The checks run in vitalsChanges' order, and the first
// refusal is the answer.
func applyVitalsChange(v *playv1.CharacterVitals, req *playv1.AdjustCharacterVitalsRequest) error {
	if !setsAnyVital(req) {
		return fieldErr("request", "must set at least one value to change")
	}
	for _, change := range vitalsChanges {
		if err := change(v, req); err != nil {
			return err
		}
	}
	return nil
}

func slotOfLevel(v *playv1.CharacterVitals, level int32) *playv1.SpellSlotUsage {
	for _, slot := range v.GetSpellSlots() {
		if slot.GetLevel() == level {
			return slot
		}
	}
	return nil
}

func inRange(field string, value, maximum int32) error {
	if value < 0 || value > maximum {
		return fieldErr(field, "must be 0 to %d", maximum)
	}
	return nil
}

func derefBool(b *bool) bool { return b != nil && *b }

func derefInt(n *int32) int32 {
	if n == nil {
		return 0
	}
	return *n
}

// carryHitPoints keeps a character's current hit points in step with a change
// of its maximum, inside the transaction that saved the sheet (RN-12): the
// maximum `before` and `after` are derived from the two stored sheets with the
// same content. A character whose current hit points are set gains what the
// maximum gained, so a wound stays a wound and a character at full health
// stays at full health; a maximum that fell never leaves the current above it.
// A character whose current hit points were never set is full whatever the
// maximum is, and needs nothing.
func (s *Service) carryHitPoints(ctx context.Context, q *charactersdb.Queries, content *rules.Content, characterID string, before, after []byte) error {
	was, err := maxima(content, characterID, before, nil)
	if err != nil {
		return err
	}
	now, err := maxima(content, characterID, after, nil)
	if err != nil {
		return err
	}
	if was.hitPoints == now.hitPoints {
		return nil
	}
	err = q.CarryHitPoints(ctx, charactersdb.CarryHitPointsParams{
		CharacterID: characterID, OldMax: i32(was.hitPoints), NewMax: i32(now.hitPoints),
		Gain: max(i32(now.hitPoints-was.hitPoints), 0), Now: s.now(),
	})
	if err != nil {
		return wrap("carry the current hit points", err)
	}
	return nil
}

// SetHitPointsMaxBonus puts the bonus Aid adds to a character's maximum hit
// points at bonus (0 or more; 0 ends the spell) inside tx, and returns the vitals
// before and after. The current hit points follow the bonus: a bonus that grew
// raises them by the same amount (the spell raises the maximum and the current
// hit points together, so a character at 0 hit points wakes up with the
// difference), and a bonus that fell takes away only what is now above the maximum
// (hit points never pass it, SRD 5.1, "Healing"), never the last hit point of
// someone who still had one. Setting the bonus it already has changes nothing, not
// even the revision. `not_found` for anything but a living, active player's
// character of the campaign. It implements play.VitalsKeeper; the caller records
// the event and resets the death saves of a character that woke up.
func (s *Service) SetHitPointsMaxBonus(ctx context.Context, tx pgx.Tx, campaignID, characterID string, bonus int32) (before, after *playv1.CharacterVitals, err error) {
	bonus = max(bonus, 0)
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return nil, nil, wrap("read rules content", err)
	}
	before, _, row, err := s.shapedVitals(ctx, tx, content, campaignID, characterID)
	if err != nil {
		return nil, nil, err
	}
	if before.GetHitPointsMaxBonus() == bonus {
		return before, before, nil
	}
	sheetMax := before.GetHitPointsMax() - before.GetHitPointsMaxBonus()
	newMax := sheetMax + bonus
	current := before.GetHitPointsCurrent()
	switch {
	case bonus > before.GetHitPointsMaxBonus():
		current += bonus - before.GetHitPointsMaxBonus()
	case current > 0:
		current = max(min(current, newMax), 1)
	}
	current = min(current, newMax)
	saved, err := s.queries.WithTx(tx).SetVitalsMaxBonus(ctx, charactersdb.SetVitalsMaxBonusParams{
		CharacterID: row.ID, HitPointsCurrent: &current, HitPointsMaxBonus: bonus, Now: s.now(),
	})
	if err != nil {
		return nil, nil, wrap("save the hit point bonus", err)
	}
	after = proto.CloneOf(before)
	after.HitPointsMaxBonus, after.HitPointsMax, after.HitPointsCurrent = bonus, newMax, current
	after.Revision, after.UpdatedAt = saved.Revision, timestamppb.New(saved.UpdatedAt)
	return before, after, nil
}

// SetExhaustion sets a character's level of exhaustion inside tx and returns the vitals
// before and after (SRD 5.1, Conditions: Exhaustion). From level 4 the hit point maximum
// is halved and the current hit points are cut to the new maximum; going back down gives
// the maximum back and never heals, so the cut stays stored. `not_found` for anything but a
// living, active player character of the campaign; `invalid_argument` for a level outside
// 0 to 6. It implements play.VitalsKeeper; the caller writes the event.
func (s *Service) SetExhaustion(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level int32) (before, after *playv1.CharacterVitals, err error) {
	if level < 0 || level > combat.MaxExhaustion {
		return nil, nil, invalidArgument(errors.New("level must be 0 to 6"))
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return nil, nil, wrap("read rules content", err)
	}
	before, _, row, err := s.shapedVitals(ctx, tx, content, campaignID, characterID)
	if err != nil {
		return nil, nil, err
	}
	if before.GetExhaustionLevel() == level {
		return before, before, nil
	}
	m, err := maxima(content, row.ID, row.Sheet, row.WildShapeBeast)
	if err != nil {
		return nil, nil, err
	}
	// The hit points the new maximum leaves. A character whose hit points were never set
	// is full: the cut stores what full is under the new maximum, so that taking the level
	// off later does not heal.
	newMax := i32(combat.ExhaustedMaxHP(m.hitPoints+int(before.GetHitPointsMaxBonus()), int(level)))
	var hitPoints *int32
	if kept := min(before.GetHitPointsCurrent(), newMax); kept < before.GetHitPointsCurrent() || (row.HitPointsCurrent == nil && newMax < i32(m.hitPoints)+before.GetHitPointsMaxBonus()) {
		hitPoints = &kept
	}
	saved, err := s.queries.WithTx(tx).SetVitalsExhaustion(ctx, charactersdb.SetVitalsExhaustionParams{CharacterID: row.ID, HitPointsCurrent: hitPoints, ExhaustionLevel: level, Now: s.now()})
	if err != nil {
		return nil, nil, wrap("save the exhaustion", err)
	}
	after = proto.CloneOf(before)
	after.ExhaustionLevel, after.HitPointsMax = level, newMax
	after.HitPointsCurrent = min(after.GetHitPointsCurrent(), newMax)
	if hitPoints == nil && before.GetHitPointsMax() < newMax && row.HitPointsCurrent == nil {
		after.HitPointsCurrent = newMax // never set: full
	}
	after.Revision, after.UpdatedAt = saved.Revision, timestamppb.New(saved.UpdatedAt)
	return before, after, nil
}

// SetArmorBase puts the base armor class an effect that lasts gives the character (Mage Armor,
// SRD 5.1), 0 for none, inside tx and returns the vitals before and after. It implements
// play.VitalsKeeper: play works the value out again in the transaction of every change to the
// character's effects, so it never drifts from them.
func (s *Service) SetArmorBase(ctx context.Context, tx pgx.Tx, campaignID, characterID string, armorClass int32) (before, after *playv1.CharacterVitals, err error) {
	if armorClass < 0 || armorClass > 60 {
		return nil, nil, invalidArgument(errors.New("armor class must be 0 to 60"))
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return nil, nil, wrap("read rules content", err)
	}
	before, _, row, err := s.shapedVitals(ctx, tx, content, campaignID, characterID)
	if err != nil {
		return nil, nil, err
	}
	if before.GetArmorClassBase() == armorClass {
		return before, before, nil
	}
	var value *int32
	if armorClass > 0 {
		value = &armorClass
	}
	saved, err := s.queries.WithTx(tx).SetVitalsArmorBase(ctx, charactersdb.SetVitalsArmorBaseParams{CharacterID: row.ID, ArmorClassBase: value, Now: s.now()})
	if err != nil {
		return nil, nil, wrap("save the armor class", err)
	}
	after = proto.CloneOf(before)
	after.ArmorClassBase = armorClass
	after.Revision, after.UpdatedAt = saved.Revision, timestamppb.New(saved.UpdatedAt)
	return before, after, nil
}
