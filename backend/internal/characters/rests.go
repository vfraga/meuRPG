package characters

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// Rests and the class resources that move points between the vitals (SRD 5.1,
// "Resting", Flexible Casting). Package play decides who may ask and when, and writes
// the history; this file changes the numbers. Every method runs inside the
// caller's transaction and reads only through it.

// restKindOf maps the API's rest kind to the rules'.
func restKindOf(kind playv1.RestKind) (string, bool) {
	switch kind {
	case playv1.RestKind_REST_KIND_SHORT:
		return rules.RestShort, true
	case playv1.RestKind_REST_KIND_LONG:
		return rules.RestLong, true
	}
	return "", false
}

// restPlan is what a rest does to one character: its vitals after, and what the
// master reads of it before.
type restPlan struct {
	after   *playv1.CharacterVitals
	preview *playv1.RestPreview
}

// planRest works out what a rest of the kind gives back to a character. choice is
// which hit dice a long rest gives back (nil: the largest first). It changes
// nothing: the plan carries the vitals after.
func planRest(kind string, v *playv1.CharacterVitals, m vitalsMax, choice rules.HitDiceUsed) (restPlan, error) {
	return planRestWith(kind, v, m, choice, nil)
}

// hookDice rolls the dice of an on_event hook's heal; nil estimates with the average.
type hookDice func(rules.DiceFormula) (int, error)

// planRestWith is planRest with the dice the rest's hooks heal rolled by roll.
func planRestWith(kind string, v *playv1.CharacterVitals, m vitalsMax, choice rules.HitDiceUsed, roll hookDice) (restPlan, error) {
	after := proto.CloneOf(v)
	restStates(kind, after) // a rest ends the states that say so, whatever else it gives
	preview := &playv1.RestPreview{
		CharacterId: v.GetCharacterId(), Name: v.GetName(),
		HitPointsCurrent: v.GetHitPointsCurrent(), HitPointsMax: v.GetHitPointsMax(),
		HitDiceBackLimit: i32(rules.LongRestHitDiceLimit(m.hitDice)),
	}
	spent := hitDiceFromProto(v.GetHitDiceUsedByDie())
	preview.HitDiceSpent = hitDiceProto(spent, m.hitDice)
	long := kind == rules.RestLong
	if long && !rules.LongRestNeedsHitPoints(int(v.GetHitPointsCurrent())) {
		// "A character must have at least 1 hit point at the start of the rest to gain
		// its benefits": nothing comes back.
		preview.NoBenefit = true
		return restPlan{after: after, preview: preview}, nil
	}
	restResources(kind, after, m, preview)
	restPactSlots(kind, after, preview)
	if rules.SpellSlotsReturnOn(kind) {
		restSpellSlots(after, preview)
	}
	if long {
		restEndAid(after)
	}
	if rules.TemporaryHitPointsEndOn(kind) {
		preview.TemporaryHitPointsLost = v.GetHitPointsTemporary()
		after.HitPointsTemporary = 0
	}
	if long {
		back, left, err := rules.HitDiceReturned(m.hitDice, spent, choice)
		if err != nil {
			return restPlan{}, link.ErrBadHitDiceChoice
		}
		preview.HitDiceBack = hitDiceProto(back, m.hitDice)
		setHitDiceUsed(after, left)
	}
	// The hooks come last: the temporary hit points of a long rest hook are given after the rest ended the old ones.
	if err := restHooks(kind, after, m, preview, roll); err != nil {
		return restPlan{}, err
	}
	return restPlan{after: after, preview: preview}, nil
}

// restResources gives back the spent resources the rest recharges.
func restResources(kind string, after *playv1.CharacterVitals, m vitalsMax, preview *playv1.RestPreview) {
	rechargeOf := map[string]string{}
	for _, r := range m.resources {
		rechargeOf[r.Key] = r.Recharge
	}
	for _, r := range after.GetResources() {
		back := resourceBack(rules.RestRecovery(rechargeOf[r.GetKey()], kind), r.GetUsed())
		if back == 0 {
			continue
		}
		entry := &playv1.RestResourceBack{Key: r.GetKey(), NamePt: r.GetNamePt(), Spent: r.GetUsed(), Total: r.GetTotal()}
		if back < r.GetUsed() {
			entry.Back = back
		}
		preview.Resources = append(preview.Resources, entry)
		r.Used -= back
	}
}

// resourceBack is how many of the used uses a rest of that recovery gives back.
func resourceBack(rec rules.Recovery, used int32) int32 {
	switch {
	case used <= 0:
		return 0
	case rec == rules.RecoverAll:
		return used
	case rec == rules.RecoverOne:
		return 1
	}
	return 0
}

// restHooks runs the character's hooks of the rest (short_rest, long_rest), in the order of its features: a hook
// that spends a resource runs only with the uses left, and one that restores uses never gives back more than are
// spent, so a rest taken again changes nothing more.
func restHooks(kind string, after *playv1.CharacterVitals, m vitalsMax, preview *playv1.RestPreview, roll hookDice) error {
	event := rules.EventShortRest
	if kind == rules.RestLong {
		event = rules.EventLongRest
	}
	for _, h := range m.hooks {
		if h.Event != event {
			continue
		}
		res, ran, err := runRestHook(h, after, roll)
		if err != nil {
			return err
		}
		if ran {
			res.SourceNamePt = m.hookNames[h.Source]
			preview.Hooks = append(preview.Hooks, res)
		}
	}
	return nil
}

// runRestHook applies one hook to the vitals after the rest, and says what it did; false when it could not be
// paid or gave nothing.
func runRestHook(h rules.EventHook, after *playv1.CharacterVitals, roll hookDice) (*playv1.HookResult, bool, error) {
	res := &playv1.HookResult{}
	if h.Spends != "" {
		r := resourceOf(after, h.Spends)
		if r == nil || r.GetTotal()-r.GetUsed() < i32(h.SpendCount) {
			return nil, false, nil
		}
		r.Used += i32(h.SpendCount)
		res.SpentResource, res.Spent = h.Spends, i32(h.SpendCount)
	}
	amount, err := hookHeal(h, int(after.GetHitPointsMax()), roll)
	if err != nil {
		return nil, false, err
	}
	if healed := combat.ApplyHeal(int(after.GetHitPointsCurrent()), int(after.GetHitPointsMax()), amount); healed.Healed > 0 {
		after.HitPointsCurrent = i32(healed.HP)
		res.Healed = i32(healed.Healed)
	}
	if h.TempHP > 0 {
		given := combat.ApplyTempHP(int(after.GetHitPointsTemporary()), h.TempHP)
		after.HitPointsTemporary = i32(given.TempHP)
		res.TempHpGained = i32(given.Gained)
	}
	if r := resourceOf(after, h.Restore); r != nil && h.RestoreCount > 0 {
		if back := i32(rules.RestoreUses(int(r.GetUsed()), h.RestoreCount)); back > 0 {
			r.Used -= back
			res.RestoredResource, res.Restored = h.Restore, back
		}
	}
	return res, res.GetHealed() > 0 || res.GetTempHpGained() > 0 || res.GetRestored() > 0 || res.GetSpent() > 0, nil
}

// hookHeal is the hit points a hook heals: its dice (rolled by roll, or the average when roll is nil) plus its share
// of the maximum.
func hookHeal(h rules.EventHook, maxHP int, roll hookDice) (int, error) {
	amount := 0
	if d := h.Heal; d != nil {
		if roll == nil {
			amount = d.Count*(d.Sides+1)/2 + d.Bonus
		} else {
			n, err := roll(*d)
			if err != nil {
				return 0, err
			}
			amount = n
		}
	}
	if h.HealPercentMax > 0 {
		amount += rules.PercentOfMax(h.HealPercentMax, maxHP)
	}
	return max(amount, 0), nil
}

// resourceOf is the resource of the vitals with the key, nil when there is none.
func resourceOf(v *playv1.CharacterVitals, key string) *playv1.ResourceUsage {
	if key == "" {
		return nil
	}
	i := slices.IndexFunc(v.GetResources(), func(r *playv1.ResourceUsage) bool { return r.GetKey() == key })
	if i < 0 {
		return nil
	}
	return v.GetResources()[i]
}

// restPactSlots gives back the spent pact magic slots, when the rest does.
func restPactSlots(kind string, after *playv1.CharacterVitals, preview *playv1.RestPreview) {
	if p := after.GetPactSlots(); p != nil && p.GetUsed() > 0 && rules.PactSlotsReturnOn(kind) {
		preview.PactSlotsBack = p.GetUsed()
		p.Used = 0
	}
}

// restSpellSlots gives back the spent spell slots of the sheet; the slots Flexible
// Casting made vanish, and a level left with none is gone from the list.
func restSpellSlots(after *playv1.CharacterVitals, preview *playv1.RestPreview) {
	var slots []*playv1.SpellSlotUsage
	for _, slot := range after.GetSpellSlots() {
		sheetTotal := slot.GetTotal() - slot.GetCreated()
		if back := min(slot.GetUsed(), sheetTotal); back > 0 {
			preview.SpellSlotsBack = append(preview.SpellSlotsBack, &playv1.SpellSlotBack{Level: slot.GetLevel(), Count: back})
		}
		preview.CreatedSlotsLost += slot.GetCreated()
		slot.Used, slot.Created, slot.Total = 0, 0, sheetTotal
		if slot.GetTotal() > 0 {
			slots = append(slots, slot)
		}
	}
	after.SpellSlots = slots
}

// restEndAid ends Ajuda, which lasts 8 hours (SRD 5.1, "Aid"): a long rest ends it, and
// the hit points that come back fill the sheet's own maximum.
func restEndAid(after *playv1.CharacterVitals) {
	after.HitPointsMax -= after.GetHitPointsMaxBonus()
	after.HitPointsMaxBonus = 0
	after.HitPointsCurrent = after.GetHitPointsMax()
}

// rollHookDice rolls the dice of a rest hook's heal in the app.
func (s *Service) rollHookDice(d rules.DiceFormula) (int, error) {
	res, err := dice.Roll(s.roller, dice.Expr{Count: d.Count, Sides: d.Sides, Modifier: d.Bonus})
	if err != nil {
		return 0, wrap("roll a hook's dice", err)
	}
	return res.Total, nil
}

// restRow is a living, active player character with its vitals and maximums.
type restRow struct {
	row  vitalsRow
	m    vitalsMax
	view *playv1.CharacterVitals
}

// restRows reads the campaign's living, active player characters inside tx.
func (s *Service) restRows(ctx context.Context, tx pgx.Tx, campaignID string) ([]restRow, error) {
	q := s.queries.WithTx(tx)
	rows, err := q.ListVitals(ctx, campaignID)
	if err != nil {
		return nil, wrap("list vitals", err)
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return nil, wrap("read rules content", err)
	}
	out := make([]restRow, 0, len(rows))
	for _, row := range rows {
		m, err := maxima(content, row.ID, row.Sheet, row.WildShapeBeast, row.ActiveStates)
		if err != nil {
			return nil, err
		}
		if err := s.liveFamiliar(ctx, q, campaignID, &row); err != nil {
			return nil, wrap("read the familiar", err)
		}
		out = append(out, restRow{row: row, m: m, view: vitalsToProto(row, m)})
	}
	return out, nil
}

// PreviewRest says what a rest of the kind would give back to each character of the
// campaign, writing nothing. It implements play.RestKeeper.
func (s *Service) PreviewRest(ctx context.Context, tx pgx.Tx, campaignID string, kind playv1.RestKind) ([]*playv1.RestPreview, error) {
	k, ok := restKindOf(kind)
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("kind must be a short or a long rest"))
	}
	rows, err := s.restRows(ctx, tx, campaignID)
	if err != nil {
		return nil, err
	}
	out := make([]*playv1.RestPreview, 0, len(rows))
	for _, r := range rows {
		plan, err := planRest(k, r.view, r.m, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, plan.preview)
	}
	return out, nil
}

// TakeRest gives each character of the campaign what the rest of the kind gives
// back and returns the vitals before and after, for the characters it changed
// (a character the rest changes nothing for is left alone: its revision stays). It
// implements play.RestKeeper. Errors are Connect errors, except link's, which package
// play words.
func (s *Service) TakeRest(ctx context.Context, tx pgx.Tx, campaignID string, req *playv1.TakeRestRequest) (before, after []*playv1.CharacterVitals, err error) {
	k, ok := restKindOf(req.GetKind())
	if !ok {
		return nil, nil, connect.NewError(connect.CodeInvalidArgument, errors.New("kind must be a short or a long rest"))
	}
	rows, err := s.restRows(ctx, tx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	choices, err := restHitDiceChoices(rows, req.GetHitDiceChoices())
	if err != nil {
		return nil, nil, err
	}
	before, after, err = s.restEveryone(ctx, tx, k, rows, choices)
	if err != nil {
		return nil, nil, err
	}
	if k == rules.RestLong && !req.GetWithoutFoodOrDrink() {
		// A long rest with food and drink takes one level of exhaustion off (SRD 5.1).
		if before, after, err = s.restEaseExhaustion(ctx, tx, campaignID, rows, before, after); err != nil {
			return nil, nil, err
		}
	}
	return before, after, nil
}

// restHitDiceChoices reads which hit dice each player chose to get back, by character:
// link.ErrBadHitDiceChoice for a character not in rows, one named twice, or a die size
// named twice in one choice.
func restHitDiceChoices(rows []restRow, given []*playv1.HitDiceChoice) (map[string]rules.HitDiceUsed, error) {
	choices := map[string]rules.HitDiceUsed{}
	for _, c := range given {
		if _, dup := choices[c.GetCharacterId()]; dup || !slices.ContainsFunc(rows, func(r restRow) bool { return r.row.ID == c.GetCharacterId() }) {
			return nil, link.ErrBadHitDiceChoice
		}
		pick, err := restHitDicePick(c)
		if err != nil {
			return nil, err
		}
		choices[c.GetCharacterId()] = pick
	}
	return choices, nil
}

// restHitDicePick is one player's choice by die size (never nil, so an empty choice gets
// no die back): link.ErrBadHitDiceChoice for a size named twice.
func restHitDicePick(c *playv1.HitDiceChoice) (rules.HitDiceUsed, error) {
	pick := rules.HitDiceUsed{}
	for _, d := range c.GetDice() {
		if _, dup := pick[int(d.GetFaces())]; dup {
			return nil, link.ErrBadHitDiceChoice
		}
		pick[int(d.GetFaces())] = int(d.GetCount())
	}
	return pick, nil
}

// restEveryone writes what the rest of the kind gives each character back, and returns
// the vitals before and after of the characters it changed, in the party's order.
func (s *Service) restEveryone(ctx context.Context, tx pgx.Tx, kind string, rows []restRow, choices map[string]rules.HitDiceUsed) (before, after []*playv1.CharacterVitals, err error) {
	q := s.queries.WithTx(tx)
	for _, r := range rows {
		plan, err := planRestWith(kind, r.view, r.m, choices[r.row.ID], s.rollHookDice)
		if err != nil {
			return nil, nil, err
		}
		if proto.Equal(plan.after, r.view) {
			continue
		}
		if err := s.writeRestPlan(ctx, q, r, plan, kind == rules.RestLong); err != nil {
			return nil, nil, err
		}
		before, after = append(before, r.view), append(after, plan.after)
	}
	return before, after, nil
}

// writeRestPlan stores one character's vitals after the rest and puts the revision on
// plan.after.
func (s *Service) writeRestPlan(ctx context.Context, q *charactersdb.Queries, r restRow, plan restPlan, long bool) error {
	saved, err := s.writeVitals(ctx, q, r.row, plan.after, long)
	if err != nil {
		return err
	}
	if r.view.GetHitPointsMaxBonus() > 0 && plan.after.GetHitPointsMaxBonus() == 0 {
		// The rest ended Ajuda: the bonus is cleared on the row, which bumps the revision once more.
		saved2, err := q.SetVitalsMaxBonus(ctx, charactersdb.SetVitalsMaxBonusParams{CharacterID: r.row.ID, HitPointsMaxBonus: 0, Now: s.now()})
		if err != nil {
			return wrap("end Aid", err)
		}
		saved.Revision, saved.UpdatedAt = saved2.Revision, saved2.UpdatedAt
	}
	if statesChanged(r.view, plan.after) {
		if err := q.SetActiveStates(ctx, charactersdb.SetActiveStatesParams{ID: r.row.ID, States: storedOf(plan.after.GetActiveStates())}); err != nil {
			return wrap("end the states", err)
		}
	}
	plan.after.Revision = saved.Revision
	plan.after.UpdatedAt = timestamppb.New(saved.UpdatedAt)
	return nil
}

// restEaseExhaustion takes one level of exhaustion off every character of rows that has
// some. The vitals after of a character the rest changed are replaced; a character it
// changed in nothing else is added at the end.
func (s *Service) restEaseExhaustion(ctx context.Context, tx pgx.Tx, campaignID string, rows []restRow, before, after []*playv1.CharacterVitals) ([]*playv1.CharacterVitals, []*playv1.CharacterVitals, error) {
	for _, r := range rows {
		if r.view.GetExhaustionLevel() < 1 {
			continue
		}
		was, now, err := s.SetExhaustion(ctx, tx, campaignID, r.row.ID, r.view.GetExhaustionLevel()-1)
		if err != nil {
			return nil, nil, err
		}
		if i := slices.IndexFunc(after, func(v *playv1.CharacterVitals) bool { return v.GetCharacterId() == r.row.ID }); i >= 0 {
			after[i] = now
		} else {
			before, after = append(before, was), append(after, now)
		}
	}
	return before, after, nil
}

// writeVitals stores the vitals a plan produced for a row: the hit points
// (current ones only when a long rest or a heal set them), the temporary ones, the
// slots used and created, the pact slots, the hit dice by size and the resources.
// What the view cannot show (the usage of a slot level or a resource the sheet lost)
// is kept as it was, except that a long rest clears every slot level.
func (s *Service) writeVitals(ctx context.Context, q *charactersdb.Queries, row vitalsRow, after *playv1.CharacterVitals, clearSlots bool) (charactersdb.UpsertVitalsRow, error) {
	used, created := restedSlots(row, after, clearSlots)
	pactUsed := conv.OrZero(row.PactSlotsUsed)
	if p := after.GetPactSlots(); p != nil {
		pactUsed = p.GetUsed()
	}
	usedResources := storedResourcesUsed(row.ResourcesUsed)
	for _, r := range after.GetResources() {
		setResourceUse(usedResources, r.GetKey(), r.GetUsed())
	}
	resourcesJSON, err := json.Marshal(usedResources)
	if err != nil {
		return charactersdb.UpsertVitalsRow{}, wrap("encode the resources", err)
	}
	hitDiceJSON, hitDiceUsed, err := encodeHitDiceUsed(hitDiceFromProto(after.GetHitDiceUsedByDie()))
	if err != nil {
		return charactersdb.UpsertVitalsRow{}, err
	}
	hitPoints := &after.HitPointsCurrent
	if row.HitPointsCurrent == nil && after.GetHitPointsCurrent() == after.GetHitPointsMax() {
		hitPoints = nil // never set and still full: stays "full, whatever the maximum becomes"
	}
	saved, err := q.UpsertVitals(ctx, charactersdb.UpsertVitalsParams{
		CharacterID: row.ID, HitPointsCurrent: hitPoints, HitPointsTemporary: after.GetHitPointsTemporary(),
		SpellSlotsUsed: used, PactSlotsUsed: pactUsed, HitDiceUsed: hitDiceUsed, HitDiceUsedByDie: hitDiceJSON,
		SpellSlotsCreated: created, ResourcesUsed: resourcesJSON, Now: s.now(),
	})
	if err != nil {
		return charactersdb.UpsertVitalsRow{}, wrap("save vitals", err)
	}
	return saved, nil
}

// restedSlots are the slots used and created, by level, to store for the vitals after:
// the stored usage of the levels the view does not show stays, unless clearSlots.
func restedSlots(row vitalsRow, after *playv1.CharacterVitals, clearSlots bool) (used, created []int32) {
	used = make([]int32, maxSpellLevel)
	copy(used, row.SpellSlotsUsed)
	created = make([]int32, maxSpellLevel)
	if clearSlots {
		clear(used)
	}
	for _, slot := range after.GetSpellSlots() {
		if l := int(slot.GetLevel()); l >= 1 && l <= maxSpellLevel {
			used[l-1], created[l-1] = slot.GetUsed(), slot.GetCreated()
		}
	}
	return used, created
}

// vitalsOfRow reads one character's vitals and maximums inside tx, or `not_found`.
func (s *Service) vitalsOfRow(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (restRow, *rules.Content, error) {
	id, ok := uid.Canonical(characterID)
	if !ok {
		return restRow{}, nil, errCharacterNotFound()
	}
	q := s.queries.WithTx(tx)
	got, err := q.GetVitals(ctx, charactersdb.GetVitalsParams{CampaignID: campaignID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return restRow{}, nil, errCharacterNotFound()
	}
	if err != nil {
		return restRow{}, nil, wrap("get vitals", err)
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return restRow{}, nil, wrap("read rules content", err)
	}
	m, err := maxima(content, got.ID, got.Sheet, got.WildShapeBeast, got.ActiveStates)
	if err != nil {
		return restRow{}, nil, err
	}
	row := vitalsRow(got)
	if err := s.liveFamiliar(ctx, q, campaignID, &row); err != nil {
		return restRow{}, nil, wrap("read the familiar", err)
	}
	return restRow{row: row, m: m, view: vitalsToProto(row, m)}, content, nil
}

// saveView writes a character's new vitals and returns them with the revision.
func (s *Service) saveView(ctx context.Context, tx pgx.Tx, r restRow, after *playv1.CharacterVitals) (*playv1.CharacterVitals, error) {
	saved, err := s.writeVitals(ctx, s.queries.WithTx(tx), r.row, after, false)
	if err != nil {
		return nil, err
	}
	after.Revision = saved.Revision
	after.UpdatedAt = timestamppb.New(saved.UpdatedAt)
	return after, nil
}

// SongOfRest is the die of the Song of Rest the party hears (SRD 5.1, Bard 2): the
// largest among the bards of the campaign's living, active characters that have the
// feature and at least 1 hit point (an unconscious bard sings nothing), 0 when
// nobody sings. Everyone who rests with such a bard hears it, the bard included
// (rules.SongOfRestFor). It implements play.RestKeeper.
func (s *Service) SongOfRest(ctx context.Context, tx pgx.Tx, campaignID string) (int, error) {
	rows, err := s.restRows(ctx, tx, campaignID)
	if err != nil {
		return 0, err
	}
	var singers []int
	for _, r := range rows {
		if r.view.GetHitPointsCurrent() >= 1 {
			singers = append(singers, r.m.songSides)
		}
	}
	return rules.SongOfRestFor(singers), nil
}

// SpendHitDie spends one hit die of the size on a short rest: face is the die
// rolled, and the character regains that plus its Constitution modifier (at least 0)
// plus extra (the face of a Song of Rest die, 0 when none applies), cut to its
// maximum. It returns the vitals before and after, the Constitution
// modifier and the hit points regained. link.ErrNoHitDiceLeft when none of the size
// is left, link.ErrBadHitDiceChoice for a size the character has not. It implements
// play.RestKeeper.
func (s *Service) SpendHitDie(ctx context.Context, tx pgx.Tx, campaignID, characterID string, faces, face, extra int) (before, after *playv1.CharacterVitals, conMod, healed int, err error) {
	r, _, err := s.vitalsOfRow(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	used := hitDiceFromProto(r.view.GetHitDiceUsedByDie())
	if !slices.ContainsFunc(r.m.hitDice, func(hd rules.HitDice) bool { return hd.Die == faces }) {
		return nil, nil, 0, 0, link.ErrBadHitDiceChoice
	}
	spent, err := rules.SpendHitDice(r.m.hitDice, used, rules.HitDiceUsed{faces: 1})
	if err != nil {
		return nil, nil, 0, 0, link.ErrNoHitDiceLeft
	}
	after = proto.CloneOf(r.view)
	setHitDiceUsed(after, spent)
	gain := rules.HitDieHeal(face, r.m.conMod) + max(extra, 0)
	after.HitPointsCurrent = min(r.view.GetHitPointsCurrent()+i32(gain), r.view.GetHitPointsMax())
	healed = int(after.GetHitPointsCurrent() - r.view.GetHitPointsCurrent())
	if after, err = s.saveView(ctx, tx, r, after); err != nil {
		return nil, nil, 0, 0, err
	}
	return r.view, after, r.m.conMod, healed, nil
}

// resourceLeft finds a resource in the vitals and the uses it has left.
func resourceLeft(v *playv1.CharacterVitals, key string) (*playv1.ResourceUsage, int32) {
	for _, r := range v.GetResources() {
		if r.GetKey() == key {
			return r, r.GetTotal() - r.GetUsed()
		}
	}
	return nil, 0
}

// slotOf finds the spell slots of a level in the vitals, nil when there are none.
func slotOf(v *playv1.CharacterVitals, level int32) *playv1.SpellSlotUsage {
	for _, slot := range v.GetSpellSlots() {
		if slot.GetLevel() == level {
			return slot
		}
	}
	return nil
}

// CreateSpellSlot turns sorcery points into a spell slot of the level (Flexible
// Casting): the points are spent from the resource and the slot is added, free, until
// the next long rest. It returns the vitals before and after and the cost. The refusals
// are link's: ErrNoResource (not a sorcerer with Font of Magic), ErrSlotLevelTooHigh,
// ErrNotEnoughPoints (a PointsError). It implements play.RestKeeper.
func (s *Service) CreateSpellSlot(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level int) (before, after *playv1.CharacterVitals, cost int, err error) {
	r, content, err := s.vitalsOfRow(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, nil, 0, err
	}
	points, left := resourceLeft(r.view, rules.SorceryPointsKey)
	if points == nil {
		return nil, nil, 0, link.ErrNoResource
	}
	cost, err = content.CreateSlotCheck(level, int(left))
	if err != nil {
		if need, ok := content.SlotCreationCost(level); ok {
			return nil, nil, 0, &link.PointsError{Err: link.ErrNotEnoughPoints, Needed: need, Available: int(left)}
		}
		return nil, nil, 0, link.ErrSlotLevelTooHigh
	}
	after = proto.CloneOf(r.view)
	p, _ := resourceLeft(after, rules.SorceryPointsKey)
	p.Used += i32(cost)
	addCreatedSlot(after, level)
	if after, err = s.saveView(ctx, tx, r, after); err != nil {
		return nil, nil, 0, err
	}
	return r.view, after, cost, nil
}

// ConvertSpellSlot expends a free spell slot of the level for as many sorcery points
// as its level (Flexible Casting), when the points do not pass the maximum. It returns
// the vitals before and after and the points gained. The refusals are link's:
// ErrNoResource, ErrNoFreeSlot, ErrPointsFull and ErrPointsOver (link.PointsError, with the
// points there are and the maximum). It implements play.RestKeeper.
func (s *Service) ConvertSpellSlot(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level int) (before, after *playv1.CharacterVitals, gain int, err error) {
	r, _, err := s.vitalsOfRow(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, nil, 0, err
	}
	points, left := resourceLeft(r.view, rules.SorceryPointsKey)
	if points == nil {
		return nil, nil, 0, link.ErrNoResource
	}
	slot := slotOf(r.view, i32(level))
	if slot == nil || slot.GetUsed() >= slot.GetTotal() {
		return nil, nil, 0, link.ErrNoFreeSlot
	}
	gain, err = rules.ConvertSlotCheck(level, int(left), int(points.GetTotal()))
	if err != nil {
		pe := &link.PointsError{Needed: int(points.GetTotal()), Available: int(left)} // the maximum, and the points now
		switch {
		case errors.Is(err, rules.ErrSorceryPointsFull):
			pe.Err = link.ErrPointsFull
		case errors.Is(err, rules.ErrSorceryPointsOver):
			pe.Err = link.ErrPointsOver
		default:
			return nil, nil, 0, link.ErrNoFreeSlot
		}
		return nil, nil, 0, pe
	}
	after = proto.CloneOf(r.view)
	p, _ := resourceLeft(after, rules.SorceryPointsKey)
	p.Used -= i32(gain)
	slotOf(after, i32(level)).Used++
	if after, err = s.saveView(ctx, tx, r, after); err != nil {
		return nil, nil, 0, err
	}
	return r.view, after, gain, nil
}

// UseSlotRecovery spends the use of a slot recovery and gives back the expended spell
// slots (SRD 5.1, Wizard, Arcane Recovery; Druid, Natural Recovery): slots maps a spell
// level to how many slots of it come back. Only the spellcasting slots count, never the
// pact slots, and only the levels of the recovery's class set the allowance. It returns the
// vitals before and after and the combined level recovered. The refusals are link's:
// ErrNoResource (the sheet lacks the feature), ErrNoUsesLeft, ErrSlotLevelTooHigh (a slot of
// the 6th level or higher), ErrSlotNotExpended and ErrSlotRecoveryOver (a PointsError: the
// combined level asked and the allowance). It implements play.RestKeeper.
func (s *Service) UseSlotRecovery(ctx context.Context, tx pgx.Tx, campaignID, characterID string, rec rules.SlotRecovery, slots map[int]int) (before, after *playv1.CharacterVitals, levels int, err error) {
	r, _, err := s.vitalsOfRow(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, nil, 0, err
	}
	use, left := resourceLeft(r.view, rec.ResourceKey)
	classLevel := r.m.recoveryLevel(rec)
	if use == nil || classLevel < 1 {
		return nil, nil, 0, link.ErrNoResource
	}
	if left < 1 {
		return nil, nil, 0, link.ErrNoUsesLeft
	}
	for level := range slots {
		if level > rules.SlotRecoveryMaxSlotLevel {
			return nil, nil, 0, link.ErrSlotLevelTooHigh
		}
	}
	for level, count := range slots {
		if slot := slotOf(r.view, i32(level)); slot == nil || int(slot.GetUsed()) < count {
			return nil, nil, 0, link.ErrSlotNotExpended
		}
	}
	levels, err = rules.SlotRecoveryCheck(slots, classLevel)
	if err != nil {
		return nil, nil, 0, &link.PointsError{Err: link.ErrSlotRecoveryOver, Needed: levels, Available: rules.SlotRecoveryAllowance(classLevel)}
	}
	after = proto.CloneOf(r.view)
	u, _ := resourceLeft(after, rec.ResourceKey)
	u.Used++
	for level, count := range slots {
		slotOf(after, i32(level)).Used -= i32(count)
	}
	if after, err = s.saveView(ctx, tx, r, after); err != nil {
		return nil, nil, 0, err
	}
	return r.view, after, levels, nil
}

// UndoCreateSpellSlot takes back a slot Flexible Casting created: the slot goes away
// (a used one counts as used no more) and the points it cost come back. It returns the
// vitals after. It implements play.RestKeeper.
func (s *Service) UndoCreateSpellSlot(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level, cost int) (*playv1.CharacterVitals, error) {
	r, _, err := s.vitalsOfRow(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	after := proto.CloneOf(r.view)
	removeCreatedSlot(after, level)
	if points, _ := resourceLeft(after, rules.SorceryPointsKey); points != nil {
		points.Used = max(points.GetUsed()-i32(cost), 0)
	}
	return s.saveView(ctx, tx, r, after)
}

// UndoConvertSpellSlot takes back a conversion of a slot into sorcery points: the slot
// is free again and the points go. It returns the vitals after. It implements
// play.RestKeeper.
func (s *Service) UndoConvertSpellSlot(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level, gain int) (*playv1.CharacterVitals, error) {
	r, _, err := s.vitalsOfRow(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	after := proto.CloneOf(r.view)
	if slot := slotOf(after, i32(level)); slot != nil {
		slot.Used = max(slot.GetUsed()-1, 0)
	}
	if points, _ := resourceLeft(after, rules.SorceryPointsKey); points != nil {
		points.Used = min(points.GetUsed()+i32(gain), points.GetTotal())
	}
	return s.saveView(ctx, tx, r, after)
}

// addCreatedSlot adds a free slot of the level to the vitals, until the next long rest.
func addCreatedSlot(after *playv1.CharacterVitals, level int) {
	if slot := slotOf(after, i32(level)); slot != nil {
		slot.Total++
		slot.Created++
		return
	}
	after.SpellSlots = append(after.SpellSlots, &playv1.SpellSlotUsage{Level: i32(level), Total: 1, Created: 1})
	slices.SortFunc(after.SpellSlots, func(a, b *playv1.SpellSlotUsage) int { return int(a.GetLevel() - b.GetLevel()) })
}

// removeCreatedSlot takes a slot addCreatedSlot made away again (a used one counts as used no more).
func removeCreatedSlot(after *playv1.CharacterVitals, level int) {
	slot := slotOf(after, i32(level))
	if slot == nil || slot.GetCreated() < 1 {
		return
	}
	slot.Total--
	slot.Created--
	slot.Used = min(slot.GetUsed(), slot.GetTotal())
	if slot.GetTotal() == 0 {
		after.SpellSlots = slices.DeleteFunc(after.SpellSlots, func(x *playv1.SpellSlotUsage) bool { return x.GetLevel() == i32(level) })
	}
}

// GiveSpellSlot adds a free spell slot of the level, until the next long rest, for a table action that turns the
// uses of a resource into a slot (the uses are spent apart, through the vitals like every spend). It returns the
// vitals after. It implements play.RestKeeper.
func (s *Service) GiveSpellSlot(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level int) (*playv1.CharacterVitals, error) {
	r, _, err := s.vitalsOfRow(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	after := proto.CloneOf(r.view)
	addCreatedSlot(after, level)
	return s.saveView(ctx, tx, r, after)
}

// UndoGiveSpellSlot takes back a slot GiveSpellSlot added. It implements play.RestKeeper.
func (s *Service) UndoGiveSpellSlot(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level int) (*playv1.CharacterVitals, error) {
	r, _, err := s.vitalsOfRow(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	after := proto.CloneOf(r.view)
	removeCreatedSlot(after, level)
	return s.saveView(ctx, tx, r, after)
}

// SpendSpellSlot expends a free spell slot of the level, for a table action that spends a slot (it heals by the
// level). link.ErrNoFreeSlot when the character has none free of that level. It returns the vitals after. It
// implements play.RestKeeper.
func (s *Service) SpendSpellSlot(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level int) (*playv1.CharacterVitals, error) {
	r, _, err := s.vitalsOfRow(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	slot := slotOf(r.view, i32(level))
	if slot == nil || slot.GetUsed() >= slot.GetTotal() {
		return nil, link.ErrNoFreeSlot
	}
	after := proto.CloneOf(r.view)
	slotOf(after, i32(level)).Used++
	return s.saveView(ctx, tx, r, after)
}

// UndoSpendSpellSlot makes the slot SpendSpellSlot expended free again. It implements play.RestKeeper.
func (s *Service) UndoSpendSpellSlot(ctx context.Context, tx pgx.Tx, campaignID, characterID string, level int) (*playv1.CharacterVitals, error) {
	r, _, err := s.vitalsOfRow(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, err
	}
	after := proto.CloneOf(r.view)
	if slot := slotOf(after, i32(level)); slot != nil {
		slot.Used = max(slot.GetUsed()-1, 0)
	}
	return s.saveView(ctx, tx, r, after)
}

// UseTableSlotRecovery spends a use of the resource of a table feature's slot_recovery effect and gives back the
// expended spell slots: slots maps a spell level to how many slots of it come back, within the highest level and the
// combined level the effect gives for the character. Only the spellcasting slots count, never the pact slots. It
// returns the vitals before and after and the combined level recovered. The refusals are link's, as UseSlotRecovery's:
// ErrNoResource (the character lacks the feature), ErrNoUsesLeft, ErrSlotLevelTooHigh, ErrSlotNotExpended and
// ErrSlotRecoveryOver (a PointsError). It implements play.RestKeeper.
func (s *Service) UseTableSlotRecovery(ctx context.Context, tx pgx.Tx, campaignID, characterID, feature string, slots map[int]int) (before, after *playv1.CharacterVitals, levels int, err error) {
	r, _, err := s.vitalsOfRow(ctx, tx, campaignID, characterID)
	if err != nil {
		return nil, nil, 0, err
	}
	i := slices.IndexFunc(r.m.recoveries, func(x rules.TableSlotRecovery) bool { return x.Source == feature })
	if i < 0 {
		return nil, nil, 0, link.ErrNoResource
	}
	rec := r.m.recoveries[i]
	use, left := resourceLeft(r.view, rec.Resource)
	if use == nil {
		return nil, nil, 0, link.ErrNoResource
	}
	if left < 1 {
		return nil, nil, 0, link.ErrNoUsesLeft
	}
	for level := range slots {
		if level > rec.MaxLevel {
			return nil, nil, 0, link.ErrSlotLevelTooHigh
		}
	}
	for level, count := range slots {
		if slot := slotOf(r.view, i32(level)); slot == nil || int(slot.GetUsed()) < count {
			return nil, nil, 0, link.ErrSlotNotExpended
		}
	}
	levels, err = rec.Check(slots)
	if err != nil {
		return nil, nil, 0, &link.PointsError{Err: link.ErrSlotRecoveryOver, Needed: levels, Available: rec.Levels}
	}
	after = proto.CloneOf(r.view)
	u, _ := resourceLeft(after, rec.Resource)
	u.Used++
	for level, count := range slots {
		slotOf(after, i32(level)).Used -= i32(count)
	}
	if after, err = s.saveView(ctx, tx, r, after); err != nil {
		return nil, nil, 0, err
	}
	return r.view, after, levels, nil
}
