package characters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
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
	m := vitalsMax{hitPoints: max(d.HitPointsMax, 0), hitDice: d.HitDice, resources: d.Resources}
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
	v := &playv1.CharacterVitals{
		CharacterId:        row.ID,
		Name:               row.Name,
		PlayerUserId:       deref(row.PlayerUserID),
		HitPointsCurrent:   i32(m.hitPoints), // fresh: full hit points
		HitPointsMax:       i32(m.hitPoints),
		HitPointsTemporary: derefInt(row.HitPointsTemporary),
		HitDiceTotal:       i32(m.hitDiceTotal),
		HitDiceUsed:        min(derefInt(row.HitDiceUsed), i32(m.hitDiceTotal)),
		Revision:           derefInt(row.Revision),
		UpdatedAt:          timestamp(row.UpdatedAt),
	}
	if row.HitPointsCurrent != nil {
		v.HitPointsCurrent = min(*row.HitPointsCurrent, v.HitPointsMax)
	}
	for k, total := range m.slots {
		if total == 0 {
			continue
		}
		var used int32
		if k < len(row.SpellSlotsUsed) {
			used = min(row.SpellSlotsUsed[k], i32(total))
		}
		v.SpellSlots = append(v.SpellSlots, &playv1.SpellSlotUsage{Level: i32(k + 1), Total: i32(total), Used: used})
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
	return s.getVitals(ctx, nil, campaignID, characterID)
}

// GetVitalsTx is GetVitals inside tx, so a change that computes from the
// vitals and writes them back (the master applying damage) reads what its own
// transaction will overwrite, never a stale copy.
func (s *Service) GetVitalsTx(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (*playv1.CharacterVitals, error) {
	return s.getVitals(ctx, tx, campaignID, characterID)
}

// getVitals reads the vitals in tx (nil: the pool).
func (s *Service) getVitals(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (*playv1.CharacterVitals, error) {
	q := s.queriesIn(tx)
	id, ok := parseUUID(characterID)
	if !ok {
		return nil, errCharacterNotFound()
	}
	row, err := q.GetVitals(ctx, charactersdb.GetVitalsParams{CampaignID: campaignID, ID: id})
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
	id, ok := parseUUID(characterID)
	if !ok {
		return nil, nil, errCharacterNotFound()
	}
	q := s.queries.WithTx(tx)
	row, err := q.GetVitals(ctx, charactersdb.GetVitalsParams{CampaignID: campaignID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, errCharacterNotFound()
	}
	if err != nil {
		return nil, nil, wrap("get vitals", err)
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return nil, nil, wrap("read rules content", err)
	}
	m, err := maxima(content, row.ID, row.Sheet, row.WildShapeBeast)
	if err != nil {
		return nil, nil, err
	}
	current := vitalsRow(row)
	if err := s.liveFamiliar(ctx, q, campaignID, &current); err != nil {
		return nil, nil, wrap("read the familiar", err)
	}
	before = vitalsToProto(current, m)
	after = proto.CloneOf(before)
	if err := applyVitalsChange(after, req); err != nil {
		return nil, nil, invalidArgument(err)
	}

	// Usage is stored as it was and changed only where req sets it: the view is cut to
	// today's sheet, and writing that back would erase the usage of slots and
	// resources the sheet has lost for now.
	used := make([]int32, maxSpellLevel)
	copy(used, row.SpellSlotsUsed)
	for _, change := range req.GetSpellSlotsUsed() {
		used[change.GetLevel()-1] = change.GetUsed()
	}
	pactUsed, hitDiceUsed := derefInt(row.PactSlotsUsed), derefInt(row.HitDiceUsed)
	if req.PactSlotsUsed != nil {
		pactUsed = after.GetPactSlots().GetUsed()
	}
	if req.HitDiceUsed != nil {
		hitDiceUsed = after.GetHitDiceUsed()
	}
	usedResources := map[string]int32{}
	_ = json.Unmarshal(row.ResourcesUsed, &usedResources)
	if usedResources == nil {
		usedResources = map[string]int32{}
	}
	for _, change := range req.GetResourcesUsed() {
		if change.GetUsed() > 0 {
			usedResources[change.GetKey()] = change.GetUsed()
		} else {
			delete(usedResources, change.GetKey())
		}
	}
	resourcesJSON, err := json.Marshal(usedResources)
	if err != nil {
		return nil, nil, wrap("encode the resources", err)
	}
	saved, err := q.UpsertVitals(ctx, charactersdb.UpsertVitalsParams{
		CharacterID:        row.ID,
		HitPointsCurrent:   after.GetHitPointsCurrent(),
		HitPointsTemporary: after.GetHitPointsTemporary(),
		SpellSlotsUsed:     used,
		PactSlotsUsed:      pactUsed,
		HitDiceUsed:        hitDiceUsed,
		ResourcesUsed:      resourcesJSON,
		Now:                s.now(),
	})
	if err != nil {
		return nil, nil, wrap("save vitals", err)
	}
	// The beast's pool is a row of its own (character_wild_shapes): 0 ended the form
	// (MR-037). UpsertVitals bumped the revision already.
	if req.WildShapeHitPointsCurrent != nil {
		if w := after.GetWildShape(); w != nil {
			err = q.SetWildShape(ctx, charactersdb.SetWildShapeParams{CharacterID: row.ID, Beast: w.GetBeastKey(), Hp: w.GetHitPointsCurrent(), Now: s.now()})
		} else {
			err = q.ClearWildShape(ctx, row.ID)
		}
		if err != nil {
			return nil, nil, wrap("save the wild shape", err)
		}
	}
	after.Revision = saved.Revision
	after.UpdatedAt = timestamppb.New(saved.UpdatedAt)
	return before, after, nil
}

// applyVitalsChange sets on v the values req sets, checking each against
// its maximum. v already has the maximums. Errors are fieldErrors, with the
// request's field names.
func applyVitalsChange(v *playv1.CharacterVitals, req *playv1.AdjustCharacterVitalsRequest) error {
	if req.HitPointsCurrent == nil && req.HitPointsTemporary == nil && len(req.GetSpellSlotsUsed()) == 0 &&
		req.PactSlotsUsed == nil && req.HitDiceUsed == nil && len(req.GetResourcesUsed()) == 0 && req.WildShapeHitPointsCurrent == nil {
		return fieldErr("request", "must set at least one value to change")
	}
	if req.WildShapeHitPointsCurrent != nil {
		w := v.GetWildShape()
		if w == nil {
			return fieldErr("wild_shape_hit_points_current", "must be unset: the character is not in a beast form")
		}
		if err := inRange("wild_shape_hit_points_current", req.GetWildShapeHitPointsCurrent(), w.GetHitPointsMax()); err != nil {
			return err
		}
		if req.GetWildShapeHitPointsCurrent() == 0 {
			v.WildShape = nil // at 0 the beast is gone: the form ends
		} else {
			w.HitPointsCurrent = req.GetWildShapeHitPointsCurrent()
		}
	}
	if req.HitPointsCurrent != nil {
		if err := inRange("hit_points_current", req.GetHitPointsCurrent(), v.GetHitPointsMax()); err != nil {
			return err
		}
		v.HitPointsCurrent = req.GetHitPointsCurrent()
	}
	if req.HitPointsTemporary != nil {
		if err := inRange("hit_points_temporary", req.GetHitPointsTemporary(), MaxTemporaryHitPoints); err != nil {
			return err
		}
		v.HitPointsTemporary = req.GetHitPointsTemporary()
	}
	seen := map[int32]bool{}
	for i, change := range req.GetSpellSlotsUsed() {
		field := fmt.Sprintf("spell_slots_used[%d]", i)
		level := change.GetLevel()
		if seen[level] {
			return fieldErr(field+".level", "repeats level %d", level)
		}
		seen[level] = true
		slot := slotOfLevel(v, level)
		if slot == nil {
			return fieldErr(field+".level", "must be a spell level the character has slots of")
		}
		if err := inRange(field+".used", change.GetUsed(), slot.GetTotal()); err != nil {
			return err
		}
		slot.Used = change.GetUsed()
	}
	if req.PactSlotsUsed != nil {
		if v.GetPactSlots() == nil {
			return fieldErr("pact_slots_used", "must be unset: the character has no pact magic slots")
		}
		if err := inRange("pact_slots_used", req.GetPactSlotsUsed(), v.GetPactSlots().GetTotal()); err != nil {
			return err
		}
		v.PactSlots.Used = req.GetPactSlotsUsed()
	}
	if req.HitDiceUsed != nil {
		if err := inRange("hit_dice_used", req.GetHitDiceUsed(), v.GetHitDiceTotal()); err != nil {
			return err
		}
		v.HitDiceUsed = req.GetHitDiceUsed()
	}
	seenResource := map[string]bool{}
	for i, change := range req.GetResourcesUsed() {
		field := fmt.Sprintf("resources_used[%d]", i)
		if seenResource[change.GetKey()] {
			return fieldErr(field+".key", "repeats a resource")
		}
		seenResource[change.GetKey()] = true
		i := slices.IndexFunc(v.GetResources(), func(r *playv1.ResourceUsage) bool { return r.GetKey() == change.GetKey() })
		if i < 0 {
			return fieldErr(field+".key", "must be a resource the character has")
		}
		if err := inRange(field+".used", change.GetUsed(), v.GetResources()[i].GetTotal()); err != nil {
			return err
		}
		v.Resources[i].Used = change.GetUsed()
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
