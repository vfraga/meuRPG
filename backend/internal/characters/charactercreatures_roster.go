package characters

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	maplink "github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// What the play module reads and writes of a character's creatures (MR-037),
// through play.CombatRoster: recording a casting, checking what a
// spell may summon, joining a combat, concentration ending, the combat writing
// hit points back, and the numbers a creature's turn is worked out from. The
// methods that write take the caller's transaction and no caller: they run
// after play's own authorization check. The rules do every number; this file
// reads and copies them.

// creatureViews turns the rows of a list into creature views.
func creatureViews[T any](rows []T, pick func(T) (charactersdb.CharacterCreature, *string)) []creatureView {
	out := make([]creatureView, 0, len(rows))
	for _, r := range rows {
		c, owner := pick(r)
		out = append(out, creatureView{CharacterCreature: c, ownerUserID: owner})
	}
	return out
}

// CharacterCreatures implements play.CombatRoster: the live creatures of the
// given characters, oldest first, inside tx (a combat's start reads them in
// its own transaction).
func (s *Service) CharacterCreatures(ctx context.Context, tx pgx.Tx, campaignID string, characterIDs []string) ([]link.Creature, error) {
	if len(characterIDs) == 0 {
		return nil, nil
	}
	rows, err := s.queriesIn(tx).ListLiveCreaturesOfCharacters(ctx, charactersdb.ListLiveCreaturesOfCharactersParams{CampaignID: campaignID, CharacterIds: characterIDs})
	if err != nil {
		return nil, wrap("list the creatures", err)
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return nil, wrap("read rules content", err)
	}
	var out []link.Creature
	for _, v := range creatureViews(rows, func(r charactersdb.ListLiveCreaturesOfCharactersRow) (charactersdb.CharacterCreature, *string) {
		return r.CharacterCreature, r.PlayerUserID
	}) {
		out = append(out, creatureOf(content, v))
	}
	return out, nil
}

// ConcentrationCreatures implements play.CombatRoster: the live creatures of a
// character that last only while it concentrates.
func (s *Service) ConcentrationCreatures(ctx context.Context, tx pgx.Tx, campaignID, characterID string) ([]link.Creature, error) {
	rows, err := s.queries.WithTx(tx).ListLiveCreaturesOnConcentration(ctx, charactersdb.ListLiveCreaturesOnConcentrationParams{CampaignID: campaignID, CharacterID: characterID})
	if err != nil {
		return nil, wrap("list the creatures that depend on a concentration", err)
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return nil, wrap("read rules content", err)
	}
	var out []link.Creature
	for _, v := range creatureViews(rows, func(r charactersdb.ListLiveCreaturesOnConcentrationRow) (charactersdb.CharacterCreature, *string) {
		return r.CharacterCreature, r.PlayerUserID
	}) {
		out = append(out, creatureOf(content, v))
	}
	return out, nil
}

// SummonCreatures implements play.CombatRoster: it records a casting. A new
// familiar replaces the old one (the SRD allows one at a time), which is
// dismissed with the reason "replaced". The creatures start at full hit points.
// Blocked with CREATURE_LIMIT when the character would keep more than 40.
func (s *Service) SummonCreatures(ctx context.Context, tx pgx.Tx, sm link.Summon) (link.SummonResult, error) {
	q := s.queries.WithTx(tx)
	var res link.SummonResult
	content, err := s.contentFor(ctx, tx, sm.CampaignID)
	if err != nil {
		return res, wrap("read rules content", err)
	}
	if sm.Source == creatureSourceSteed {
		if bonded, err := s.reviveBondedSteed(ctx, q, content, sm); err != nil || bonded != nil {
			res.Created = bonded
			return res, err
		}
	}
	if res.Replaced, err = s.replaceForSummon(ctx, q, content, sm); err != nil {
		return res, err
	}
	if err := checkCreatureRoom(ctx, q, sm); err != nil {
		return res, err
	}
	if err := checkSummonLive(ctx, q, sm); err != nil {
		return res, err
	}
	if res.Created, err = s.insertSummoned(ctx, q, content, sm); err != nil {
		return res, err
	}
	return res, setSummonedOwner(ctx, q, sm.CampaignID, res.Created)
}

// replaceForSummon dismisses what the casting replaces and returns it: the old
// familiar for a new one ("replaced"), the creatures of the same table spell when
// it replaces its own ("replaced"), and for a concentration spell the creatures
// of the old concentration ("concentration").
func (s *Service) replaceForSummon(ctx context.Context, q *charactersdb.Queries, content *rules.Content, sm link.Summon) ([]link.Creature, error) {
	var out []link.Creature
	if sm.Source == creatureSourceFamiliar {
		old, err := q.ListLiveFamiliars(ctx, charactersdb.ListLiveFamiliarsParams{CampaignID: sm.CampaignID, CharacterID: sm.CharacterID})
		if err != nil {
			return out, wrap("list the familiars", err)
		}
		replaced, err := s.dismissReplaced(ctx, q, content, sm.CampaignID, creatureViews(old, func(r charactersdb.ListLiveFamiliarsRow) (charactersdb.CharacterCreature, *string) {
			return r.CharacterCreature, r.PlayerUserID
		}), "replaced", "dismiss the old familiar")
		if out = append(out, replaced...); err != nil {
			return out, err
		}
	}
	if sm.ReplacesSame {
		replaced, err := s.replaceSameSummon(ctx, q, content, sm)
		if out = append(out, replaced...); err != nil {
			return out, err
		}
	}
	if sm.Concentration {
		// A new concentration spell ends the old one, and the creatures that lasted
		// only while it did (RN-22), in or out of a combat.
		replaced, err := s.replaceConcentration(ctx, q, content, sm)
		if out = append(out, replaced...); err != nil {
			return out, err
		}
	}
	return out, nil
}

// replaceSameSummon dismisses the live creatures of the table spell the casting is of.
func (s *Service) replaceSameSummon(ctx context.Context, q *charactersdb.Queries, content *rules.Content, sm link.Summon) ([]link.Creature, error) {
	old, err := q.ListLiveCreaturesOfSummon(ctx, charactersdb.ListLiveCreaturesOfSummonParams{CampaignID: sm.CampaignID, CharacterID: sm.CharacterID, SummonKey: sm.SummonKey})
	if err != nil {
		return nil, wrap("list the creatures of the spell", err)
	}
	return s.dismissReplaced(ctx, q, content, sm.CampaignID, creatureViews(old, func(r charactersdb.ListLiveCreaturesOfSummonRow) (charactersdb.CharacterCreature, *string) {
		return r.CharacterCreature, r.PlayerUserID
	}), "replaced", "dismiss the old creatures of the spell")
}

// replaceConcentration dismisses the live creatures that depend on the caster's concentration.
func (s *Service) replaceConcentration(ctx context.Context, q *charactersdb.Queries, content *rules.Content, sm link.Summon) ([]link.Creature, error) {
	old, err := q.ListLiveCreaturesOnConcentration(ctx, charactersdb.ListLiveCreaturesOnConcentrationParams{CampaignID: sm.CampaignID, CharacterID: sm.CharacterID})
	if err != nil {
		return nil, wrap("list the creatures that depend on a concentration", err)
	}
	return s.dismissReplaced(ctx, q, content, sm.CampaignID, creatureViews(old, func(r charactersdb.ListLiveCreaturesOnConcentrationRow) (charactersdb.CharacterCreature, *string) {
		return r.CharacterCreature, r.PlayerUserID
	}), "concentration", "dismiss the old concentration's creatures")
}

// dismissReplaced dismisses the creatures with the reason (what names the
// write in an error) and returns them as they were; none is no write.
func (s *Service) dismissReplaced(ctx context.Context, q *charactersdb.Queries, content *rules.Content, campaignID string, old []creatureView, reason, what string) ([]link.Creature, error) {
	if len(old) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(old))
	out := make([]link.Creature, 0, len(old))
	for _, o := range old {
		ids = append(ids, o.ID)
		cr := creatureOf(content, o)
		cr.DismissedReason = reason
		out = append(out, cr)
	}
	if _, err := q.DismissCreatures(ctx, charactersdb.DismissCreaturesParams{CampaignID: campaignID, Ids: ids, Reason: reason, At: s.now()}); err != nil {
		return out, wrap(what, err)
	}
	return out, nil
}

// checkCreatureRoom blocks a casting that would leave the character with more
// than maxCreaturesPerCharacter creatures.
func checkCreatureRoom(ctx context.Context, q *charactersdb.Queries, sm link.Summon) error {
	live, err := q.CountLiveCreaturesOfCharacter(ctx, charactersdb.CountLiveCreaturesOfCharacterParams{CampaignID: sm.CampaignID, CharacterID: sm.CharacterID})
	if err != nil {
		return wrap("count the creatures", err)
	}
	if int(live)+len(sm.Creatures) > maxCreaturesPerCharacter {
		return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CREATURE_LIMIT, sm.CharacterID)
	}
	return nil
}

// checkSummonLive blocks a casting of a table spell that would leave the character with more of its creatures
// than the spell's max_live (the creatures the casting replaced are gone by now). It counts the character's live
// creatures of the spell, in any combat or none, and runs in the casting's transaction, so nothing is made on a refusal.
func checkSummonLive(ctx context.Context, q *charactersdb.Queries, sm link.Summon) error {
	if sm.MaxLive <= 0 || sm.SummonKey == "" {
		return nil
	}
	live, err := q.ListLiveCreaturesOfSummon(ctx, charactersdb.ListLiveCreaturesOfSummonParams{CampaignID: sm.CampaignID, CharacterID: sm.CharacterID, SummonKey: sm.SummonKey})
	if err != nil {
		return wrap("count the creatures of the spell", err)
	}
	if len(live)+len(sm.Creatures) > sm.MaxLive {
		return errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CREATURE_LIMIT, sm.CharacterID)
	}
	return nil
}

// insertSummoned writes the casting's creatures at full hit points, in the
// casting's order. A creature with formulas gets its numbers worked out for its
// owner and the slot of the casting. A creature that rolls its own initiative has
// a summon group of its own; the concentration it depends on is the casting's.
func (s *Service) insertSummoned(ctx context.Context, q *charactersdb.Queries, content *rules.Content, sm link.Summon) ([]link.Creature, error) {
	var out []link.Creature
	labels := creatureNames(content, sm.Creatures)
	at := s.now()
	owner := &lazyOwner{s: s, q: q, content: content, campaignID: sm.CampaignID, characterID: sm.CharacterID}
	for i, spec := range sm.Creatures {
		stat, hp, err := owner.numbersOf(ctx, content, spec.MonsterKey, sm.Slot, spec.MinInt, spec.CreatureType)
		if err != nil {
			return out, err
		}
		p := summonedParams(sm, spec, labels[i], stat, hp)
		// Each a microsecond after the last: the casting's order is the list's order, so "the
		// first creature of the group" is the same wherever it is read.
		p.CreatedAt = at.Add(time.Duration(i) * time.Microsecond)
		row, err := q.InsertCharacterCreature(ctx, p)
		if err != nil {
			return out, wrap("insert a creature", err)
		}
		out = append(out, creatureOf(content, creatureView{CharacterCreature: row}))
	}
	return out, nil
}

// summonedParams is the row of one creature of the casting.
func summonedParams(sm link.Summon, spec link.CreatureSpec, name string, stat []byte, hp int32) charactersdb.InsertCharacterCreatureParams {
	p := charactersdb.InsertCharacterCreatureParams{
		CampaignID: sm.CampaignID, CharacterID: sm.CharacterID, MonsterKey: spec.MonsterKey, Name: name, Source: sm.Source,
		Attack: spec.Attack, SummonGroupID: sm.GroupID, HpCurrent: hp, HpMax: hp, StatBlock: stat,
	}
	if sm.Concentration {
		p.ConcentrationCastID = &sm.GroupID
	}
	if sm.OwnInitiative {
		p.SummonGroupID = uuid.New().String()
	}
	if sm.SummonKey != "" {
		p.SummonKey = &sm.SummonKey
	}
	if sm.Source == creatureSourceSteed {
		p.CreatureType, p.Bonded = spec.CreatureType, true
	}
	return p
}

// setSummonedOwner gives the new creatures their owner's player, read with the
// first of them.
func setSummonedOwner(ctx context.Context, q *charactersdb.Queries, campaignID string, created []link.Creature) error {
	owner, err := q.GetCharacterCreature(ctx, charactersdb.GetCharacterCreatureParams{CampaignID: campaignID, ID: created[0].ID})
	if err != nil {
		return wrap("read the owner", err)
	}
	for i := range created {
		created[i].OwnerUserID = conv.OrZero(owner.PlayerUserID)
	}
	return nil
}

// creatureNames gives each creature of a casting its name: the one the player
// chose, or the creature's Portuguese name, numbered when several of the
// casting share it ("Lobo atroz 1", "Lobo atroz 2").
func creatureNames(c *rules.Content, specs []link.CreatureSpec) []string {
	total := map[string]int{}
	for _, sp := range specs {
		if sp.Name == "" {
			total[sp.MonsterKey]++
		}
	}
	seen := map[string]int{}
	out := make([]string, len(specs))
	for i, sp := range specs {
		if sp.Name != "" {
			out[i] = sp.Name
			continue
		}
		base := c.NamePT(sp.MonsterKey)
		if base == "" {
			base = sp.MonsterKey
		}
		if total[sp.MonsterKey] > 1 {
			seen[sp.MonsterKey]++
			suffix := " " + strconv.Itoa(seen[sp.MonsterKey])
			if r := []rune(base); len(r) > maxCreatureName-len(suffix) {
				base = string(r[:maxCreatureName-len(suffix)])
			}
			base += suffix
		}
		if n, err := names.Clean(base, maxCreatureName); err == nil {
			base = n
		}
		out[i] = base
	}
	return out
}

// DismissCreatures implements play.CombatRoster: the live ones of these
// creatures leave their owners with the reason ("concentration", "defeated",
// "replaced"...); it returns the IDs it dismissed.
func (s *Service) DismissCreatures(ctx context.Context, tx pgx.Tx, campaignID string, ids []string, reason string, at time.Time) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out, err := s.queries.WithTx(tx).DismissCreatures(ctx, charactersdb.DismissCreaturesParams{CampaignID: campaignID, Ids: ids, Reason: reason, At: at})
	if err != nil {
		return nil, wrap("dismiss creatures", err)
	}
	return out, nil
}

// ReviveCreatures implements play.CombatRoster: the creatures dismissed for the
// reason are back (an undo); it returns the IDs it revived.
func (s *Service) ReviveCreatures(ctx context.Context, tx pgx.Tx, campaignID string, ids []string, reason string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out, err := s.queries.WithTx(tx).ReviveCreatures(ctx, charactersdb.ReviveCreaturesParams{CampaignID: campaignID, Ids: ids, Reason: reason})
	if err != nil {
		return nil, wrap("revive creatures", err)
	}
	return out, nil
}

// DeleteCreatures implements play.CombatRoster: the creatures a casting made
// are gone for good (the master's undo of that casting).
func (s *Service) DeleteCreatures(ctx context.Context, tx pgx.Tx, campaignID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := s.queries.WithTx(tx).DeleteCreatures(ctx, charactersdb.DeleteCreaturesParams{CampaignID: campaignID, Ids: ids}); err != nil {
		return wrap("delete creatures", err)
	}
	return nil
}

// SyncCreatures implements play.CombatRoster: it keeps the creatures in step
// with the combat that holds them. A creature whose combatant is defeated (0
// hit points) is dismissed with the reason "defeated" ("disappeared" for a steed) and one that was
// dismissed as defeated and is up again (a heal, an undo) is back, each with
// the hit points of its combatant. It returns who changed.
func (s *Service) SyncCreatures(ctx context.Context, tx pgx.Tx, campaignID string, states []link.CreatureState, at time.Time) (link.CreatureChanges, error) {
	var out link.CreatureChanges
	if len(states) == 0 {
		return out, nil
	}
	q := s.queries.WithTx(tx)
	byID, err := creaturesOfStates(ctx, q, campaignID, states)
	if err != nil {
		return out, err
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return out, wrap("read rules content", err)
	}
	for _, st := range states {
		v, ok := byID[st.ID]
		if !ok {
			continue
		}
		if err := syncCreature(ctx, q, content, campaignID, v, st, at, &out); err != nil {
			return out, err
		}
	}
	return out, nil
}

// creaturesOfStates reads the creatures a combat names, live or not, by ID.
func creaturesOfStates(ctx context.Context, q *charactersdb.Queries, campaignID string, states []link.CreatureState) (map[string]creatureView, error) {
	ids := make([]string, 0, len(states))
	for _, st := range states {
		ids = append(ids, st.ID)
	}
	rows, err := q.ListCreaturesByIDs(ctx, charactersdb.ListCreaturesByIDsParams{CampaignID: campaignID, Ids: ids})
	if err != nil {
		return nil, wrap("read the creatures of a combat", err)
	}
	byID := make(map[string]creatureView, len(rows))
	for _, r := range rows {
		byID[r.CharacterCreature.ID] = creatureView{CharacterCreature: r.CharacterCreature, ownerUserID: r.PlayerUserID}
	}
	return byID, nil
}

// syncCreature puts one creature in step with its combatant, adding it to out
// when it changed: a live one defeated is dismissed, one defeated before and up
// again is revived; any other is left as it is.
func syncCreature(ctx context.Context, q *charactersdb.Queries, content *rules.Content, campaignID string, v creatureView, st link.CreatureState, at time.Time, out *link.CreatureChanges) error {
	switch {
	case st.Defeated && v.live():
		if _, err := q.DismissCreatures(ctx, charactersdb.DismissCreaturesParams{CampaignID: campaignID, Ids: []string{v.ID}, Reason: fallenReason(v.Source), At: at}); err != nil {
			return wrap("dismiss a defeated creature", err)
		}
		v.HpCurrent = 0
		out.Dismissed = append(out.Dismissed, creatureOf(content, v))
	case !st.Defeated && !v.live() && v.DismissedReason != nil && *v.DismissedReason == fallenReason(v.Source):
		return reviveCreature(ctx, q, content, campaignID, v, st, out)
	}
	return nil
}

// reviveCreature brings a defeated creature back with its combatant's hit
// points, clamped to 0 and to its maximum.
func reviveCreature(ctx context.Context, q *charactersdb.Queries, content *rules.Content, campaignID string, v creatureView, st link.CreatureState, out *link.CreatureChanges) error {
	hp := int32(min(max(st.HP, 0), int(v.HpMax))) //nolint:gosec // clamped to the creature's maximum
	if _, err := q.ReviveCreatures(ctx, charactersdb.ReviveCreaturesParams{CampaignID: campaignID, Ids: []string{v.ID}, Reason: fallenReason(v.Source)}); err != nil {
		return wrap("revive a creature", err)
	}
	if err := q.SetCharacterCreatureHitPoints(ctx, charactersdb.SetCharacterCreatureHitPointsParams{CampaignID: campaignID, ID: v.ID, HpCurrent: hp}); err != nil {
		return wrap("restore a creature's hit points", err)
	}
	v.HpCurrent = hp
	out.Revived = append(out.Revived, creatureOf(content, v))
	return nil
}

// WriteBackCreatures implements play.CombatRoster: a combat's end (or a
// creature leaving it) writes each creature's hit points back. A creature at 0
// is dismissed ("defeated", "disappeared" for a steed) if it is not already.
func (s *Service) WriteBackCreatures(ctx context.Context, tx pgx.Tx, campaignID string, states []link.CreatureState, at time.Time) error {
	if len(states) == 0 {
		return nil
	}
	q := s.queries.WithTx(tx)
	for _, st := range states {
		id, ok := uid.Canonical(st.ID)
		if !ok {
			continue
		}
		row, err := q.GetCharacterCreature(ctx, charactersdb.GetCharacterCreatureParams{CampaignID: campaignID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			continue // dismissed for good meanwhile
		}
		if err != nil {
			return wrap("read a creature", err)
		}
		hp := int32(min(max(st.HP, 0), int(row.CharacterCreature.HpMax))) //nolint:gosec // clamped to the creature's maximum
		if st.Defeated {
			if _, err := q.DismissCreatures(ctx, charactersdb.DismissCreaturesParams{CampaignID: campaignID, Ids: []string{id}, Reason: fallenReason(row.CharacterCreature.Source), At: at}); err != nil {
				return wrap("dismiss a defeated creature", err)
			}
			continue
		}
		if err := q.SetCharacterCreatureHitPoints(ctx, charactersdb.SetCharacterCreatureHitPointsParams{CampaignID: campaignID, ID: id, HpCurrent: hp}); err != nil {
			return wrap("write a creature's hit points back", err)
		}
	}
	return nil
}

// summonCharacter reads the living player's character that casts and its
// build; `not_found` for any other.
func (s *Service) summonCharacter(ctx context.Context, tx pgx.Tx, campaignID, characterID string) (rules.Build, error) {
	id, ok := uid.Canonical(characterID)
	if !ok {
		return rules.Build{}, errCharacterNotFound()
	}
	rows, err := s.queriesIn(tx).ListCombatCharacters(ctx, charactersdb.ListCombatCharactersParams{CampaignID: campaignID, Ids: []string{id}})
	if err != nil {
		return rules.Build{}, s.dbError(ctx, "read a character for a summon", err)
	}
	if len(rows) == 0 || rows[0].Kind != kindPlayer {
		return rules.Build{}, errCharacterNotFound()
	}
	sheet, err := loadSheet(rows[0].ID, rows[0].Sheet)
	if err != nil {
		return rules.Build{}, s.dbError(ctx, "read a character for a summon", err)
	}
	if sheet.GetFull() == nil {
		return rules.Build{}, errCharacterNotFound()
	}
	return buildOf(sheet.GetFull()), nil
}

// CheckSummon implements play.CombatRoster: what a casting of the spell by the
// character may do. circle is the slot's circle, 0 for the spell's own (a
// ritual). It returns what the spell and the sheet say (the spell is on the
// sheet, can be cast as a ritual...) and the creatures of the choice. A choice
// the spell does not allow comes back as one of rules.ErrSummonOption,
// ErrSummonCount, ErrSummonCreature or ErrSummonCircle, a spell that does not
// summon as rules.ErrNotSummonSpell, for play to turn into its own reasons.
func (s *Service) CheckSummon(ctx context.Context, tx pgx.Tx, campaignID, characterID, spellKey string, circle, option int, keys []string, creatureType string) (link.SummonSpell, error) {
	b, err := s.summonCharacter(ctx, tx, campaignID, characterID)
	if err != nil {
		return link.SummonSpell{}, err
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return link.SummonSpell{}, wrap("read rules content", err)
	}
	det, ok := content.SpellDetails(spellKey)
	if !ok {
		return link.SummonSpell{}, connect.NewError(connect.CodeNotFound, errUnknownSpell)
	}
	derived := rules.Derive(b, content)
	if isFeatureSummon(content, spellKey) && !slices.ContainsFunc(derived.GrantedSpells, func(g rules.GrantedSpell) bool { return g.Spell.Key == spellKey }) {
		// The spell of a feature's summon is the feature's: a character without the grant gets the answer of an unknown spell,
		// so a ritual cast cannot probe for the key.
		return link.SummonSpell{}, connect.NewError(connect.CodeNotFound, errUnknownSpell)
	}
	if circle == 0 {
		circle = det.Spell.Level
	}
	choices, err := content.SummonOptions(spellKey, circle, b)
	if err != nil {
		return link.SummonSpell{}, err
	}
	out := link.SummonSpell{
		Ritual: choices.Ritual, Concentration: choices.Concentration, CastingUnit: choices.CastingTime.Unit, Level: det.Spell.Level,
	}
	st := s.summonStanding(derived, spellKey, choices.Ritual)
	out.Known, out.Prepared, out.CanRitual = st.known, st.prepared, st.canRitual
	made, err := content.CheckSummon(spellKey, circle, b, rules.SummonPick{Option: option, Creatures: keys, CreatureType: creatureType})
	if err != nil {
		return out, err
	}
	for _, m := range made {
		out.Creatures = append(out.Creatures, link.SummonedForm{MonsterKey: m.Key, Attack: m.Attack, CreatureType: m.CreatureType, Mount: m.Mount, MinInt: m.MinInt})
	}
	if err := s.checkBondedSteed(ctx, tx, campaignID, characterID, spellKey, out.Creatures); err != nil {
		return out, err
	}
	if rule, table := content.SummonRule(spellKey); table {
		out.Table, out.TurnMode, out.MaxLive, out.ReplacesSame = true, rule.TurnMode, rule.MaxLive, rule.ReplacesSame
	}
	out.Feature = isFeatureSummon(content, spellKey)
	return out, nil
}

// summonStanding is what a character's sheet says about a summoning spell: the
// spell is on it (known), ready (prepared) and castable as a ritual. CheckSummon
// and GetSummonOptions share it, so the read and the cast agree with the cast menu.
type summonStanding struct{ known, prepared, canRitual bool }

// summonStanding is the sheet's standing for the spell; ritualSpell is whether the
// spell is a ritual at all.
func (s *Service) summonStanding(d rules.Derived, spellKey string, ritualSpell bool) summonStanding {
	st := d.SpellStanding(spellKey, ritualSpell)
	return summonStanding{known: st.Known, prepared: st.Prepared, canRitual: st.CanRitual}
}

// creatureDerived is the stat block of a creature as the combat reads it: the
// SRD creature's numbers, and what the spell lets it attack with. A creature
// that may not attack (a familiar) has no attacks and no Attack action; one that
// attacks only with its reaction keeps its attacks for the opportunity attack.
// No creature casts a spell here.
func creatureDerived(content *rules.Content, key, attack string) (rules.Derived, bool) {
	return creatureDerivedFrom(content, key, attack, nil)
}

// creatureDerivedFrom is creatureDerived for a creature with the snapshot of its numbers (nil for none).
func creatureDerivedFrom(content *rules.Content, key, attack string, statBlock []byte) (rules.Derived, bool) {
	d, ok := monsterDerivedOf(content, key, statBlock)
	if !ok {
		return rules.Derived{}, false
	}
	d.StandardActions = slices.DeleteFunc(slices.Clone(d.StandardActions), func(a rules.Action) bool {
		return a.Key == "standard:cast-a-spell" || (attack != "full" && a.Key == "standard:attack")
	})
	if attack == "none" {
		d.Attacks = nil
	}
	d.Features = withCreatureType(content, key, d.Features)
	if kind := storedTypeOf(key, statBlock); kind != "" { // a steed: the type the player picked is the one the combat reads
		d.Features = append(slices.DeleteFunc(slices.Clone(d.Features), func(f rules.Feature) bool { return strings.HasPrefix(f.Key, creatureTypePrefix) }),
			rules.Feature{Key: creatureTypePrefix + kind, Name: kind, Source: key})
	}
	return d, true
}

// storedTypeOf is the type the snapshot of a creature says it was given, "" for none.
func storedTypeOf(key string, statBlock []byte) string {
	var r rules.ResolvedCreature
	if len(statBlock) == 0 || json.Unmarshal(statBlock, &r) != nil || r.Key != key {
		return ""
	}
	return r.CreatureType
}

// CreatureSheet implements play.CombatRoster: what an attack needs from a
// creature's stat block (its armor class, its attacks, the standard actions);
// a creature with a snapshot reads the snapshot's numbers.
// False for a key that is not a creature of the content (SRD or the table's).
func (s *Service) CreatureSheet(ctx context.Context, tx pgx.Tx, campaignID, creatureID, monsterKey, attack string) (link.Sheet, bool, error) {
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return link.Sheet{}, false, wrap("read rules content", err)
	}
	stat, err := s.statBlockOf(ctx, tx, content, campaignID, creatureID, monsterKey)
	if err != nil {
		return link.Sheet{}, false, err
	}
	d, ok := creatureDerivedFrom(content, monsterKey, attack, stat)
	if !ok {
		return link.Sheet{}, false, nil
	}
	out := link.Sheet{ArmorClass: d.ArmorClass, AttacksPerAction: max(d.AttacksPerAction, 1), Senses: senseRanges(d.Senses), Traits: traitsOf(content, d)}
	out.Traits.Intelligence = intelligenceOf(d)
	for _, a := range d.Attacks {
		name := a.NamePT
		if name == "" {
			name = a.Name
		}
		out.Attacks = append(out.Attacks, link.Attack{
			Key: a.Key, Name: name, Spell: a.Kind == "spell", ToHit: a.AttackBonus,
			DiceCount: a.DamageDice.Count, DiceSides: a.DamageDice.Sides, DiceBonus: a.DamageDice.Bonus,
			DamageType: a.DamageType, RangeFt: a.RangeFt, LongRangeFt: a.LongRangeFt, Melee: a.Melee, ReachFt: a.ReachFt,
			Ability: string(a.Ability), Weapon: a.Kind == "weapon", Magical: a.Magical, Silvered: a.Silvered, Adamantine: a.Adamantine,
		})
	}
	for _, a := range d.StandardActions {
		out.Actions = append(out.Actions, link.Action{Key: a.Key, Name: a.NamePT})
	}
	return out, true, nil
}

// CreatureTurnOptions implements play.CombatRoster: what the creature can do
// now, from its stat block and what it used this turn. A creature that attacks
// only with its reaction shows its attacks disabled (REACTION_ONLY): the app
// offers them when a move leaves its reach. False for a key that is not a
// creature of the content (SRD or the table's).
func (s *Service) CreatureTurnOptions(ctx context.Context, tx pgx.Tx, campaignID, creatureID, monsterKey, attack string, turn link.Turn) (*rulesv1.TurnOptions, bool, error) {
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return nil, false, wrap("read rules content", err)
	}
	stat, err := s.statBlockOf(ctx, tx, content, campaignID, creatureID, monsterKey)
	if err != nil {
		return nil, false, err
	}
	d, ok := creatureDerivedFrom(content, monsterKey, attack, stat)
	if !ok {
		return nil, false, nil
	}
	d.SpeedWalkFt = turn.SpeedFt
	opts := combat.Options(d, combat.TurnState{
		ActionUsed: turn.ActionUsed, BonusActionUsed: turn.BonusActionUsed, ReactionUsed: turn.ReactionUsed,
		MovementUsedFt: turn.MovementUsedFt, Dashed: turn.Dashed, AttacksMade: turn.AttacksMade,
	}, combat.Usage{})
	out := turnOptionsToProto(opts)
	if attack == "reaction" {
		for _, a := range out.Attacks {
			a.Enabled = false
			a.Reason = &rulesv1.DisabledReason{Code: rulesv1.DisabledReasonCode_DISABLED_REASON_CODE_REACTION_ONLY}
		}
	}
	return out, true, nil
}

// CreatureSave implements play.CombatRoster: a creature's saving throw bonus
// against an ability ("dex"), from its stat block (the listed bonus, or the
// ability modifier).
func (s *Service) CreatureSave(ctx context.Context, tx pgx.Tx, campaignID, creatureID, monsterKey, ability string) (link.Save, error) {
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return link.Save{}, wrap("read rules content", err)
	}
	stat, err := s.statBlockOf(ctx, tx, content, campaignID, creatureID, monsterKey)
	if err != nil {
		return link.Save{}, err
	}
	d, ok := monsterDerivedOf(content, monsterKey, stat)
	if !ok {
		return link.Save{}, nil
	}
	for _, st := range d.SavingThrows {
		if string(st.Ability) == ability {
			return link.Save{Bonus: st.Bonus, Known: true}, nil
		}
	}
	return link.Save{}, nil
}

// CreatureCheck implements play.CombatRoster: a creature's bonus in a skill or ability
// check from its stat block, with the passive score of the skills that have one. Known is
// false for a key that is not a skill or an ability the stat block gives.
func (s *Service) CreatureCheck(ctx context.Context, tx pgx.Tx, campaignID, creatureID, monsterKey, key string) (link.SceneOption, error) {
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return link.SceneOption{}, wrap("read rules content", err)
	}
	stat, err := s.statBlockOf(ctx, tx, content, campaignID, creatureID, monsterKey)
	if err != nil {
		return link.SceneOption{}, err
	}
	d, ok := monsterDerivedOf(content, monsterKey, stat)
	if !ok {
		return link.SceneOption{}, nil
	}
	opts, err := rules.SceneOptions(d, []rules.SceneAction{{Key: key}})
	if err != nil {
		return link.SceneOption{}, nil //nolint:nilerr // a skill the scene rules do not list for the creature has no number: the caller reads the zero option as unknown
	}
	o := opts[0]
	return link.SceneOption{Known: true, CheckName: o.NamePT, Bonus: o.Bonus, Passive: o.Passive, HasPassive: o.HasPassive}, nil
}

// DamageModifiers implements play.CombatRoster: the damage types the target
// takes double, half or none of, from its stat block. A creature's key is given;
// an NPC made from a creature has it on its basic sheet; anyone else (a player's
// character, an NPC typed by hand) has none. The entries limited to nonmagical attacks,
// with or without an exception for silvered or adamantine weapons, are kept apart.
func (s *Service) DamageModifiers(ctx context.Context, tx pgx.Tx, campaignID, characterID, monsterKey string) (combat.TypeModifiers, error) {
	if monsterKey == "" {
		id, ok := uid.Canonical(characterID)
		if !ok {
			return combat.TypeModifiers{}, nil
		}
		rows, err := s.queriesIn(tx).ListCombatCharacters(ctx, charactersdb.ListCombatCharactersParams{CampaignID: campaignID, Ids: []string{id}})
		if err != nil {
			return combat.TypeModifiers{}, s.dbError(ctx, "read a character for a damage", err)
		}
		if len(rows) == 0 || rows[0].Kind == kindPlayer {
			return combat.TypeModifiers{}, nil
		}
		sheet, err := loadSheet(rows[0].ID, rows[0].Sheet)
		if err != nil {
			return combat.TypeModifiers{}, s.dbError(ctx, "read a character for a damage", err)
		}
		if monsterKey = sheet.GetBasic().GetMonsterKey(); monsterKey == "" {
			return combat.TypeModifiers{}, nil
		}
	}
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return combat.TypeModifiers{}, wrap("read rules content", err)
	}
	c, ok := content.CreatureByKey(monsterKey)
	if !ok {
		return combat.TypeModifiers{}, nil
	}
	return typeModifiersOf(c, monsterKey), nil
}

// typeModifiersOf reads the damage lists of a stat block: the plain entries, and the
// resistances and immunities the SRD limits to nonmagical attacks (and those that also
// exempt a silvered or an adamantine weapon), kept apart. An entry with any other
// condition ("damage from spells") is left to the master.
func typeModifiersOf(c rules.Creature, monsterKey string) combat.TypeModifiers {
	return combat.TypeModifiers{
		Vulnerable: damageTypesOf(c.Vulnerabilities, ""), Resistant: damageTypesOf(c.Resistances, ""), Immune: damageTypesOf(c.Immunities, ""),
		ResistantNonmagical: damageTypesOf(c.Resistances, rules.QualifierNonmagical), ImmuneNonmagical: damageTypesOf(c.Immunities, rules.QualifierNonmagical),
		ResistantUnlessSilvered: damageTypesOf(c.Resistances, rules.QualifierNonmagicalUnlessSilvered), ImmuneUnlessSilvered: damageTypesOf(c.Immunities, rules.QualifierNonmagicalUnlessSilvered),
		ResistantUnlessAdamantine: damageTypesOf(c.Resistances, rules.QualifierNonmagicalUnlessAdamantine), ImmuneUnlessAdamantine: damageTypesOf(c.Immunities, rules.QualifierNonmagicalUnlessAdamantine),
		Source: monsterKey,
	}
}

// damageTypesOf lists the damage types of the entries with this qualifier: "" for the
// entries that carry no condition at all.
func damageTypesOf(mods []rules.CreatureDamageMod, qualifier string) []string {
	var out []string
	for _, m := range mods {
		if m.Qualifier != qualifier || (qualifier == "" && m.Note != "") {
			continue
		}
		for _, t := range m.Types {
			out = append(out, t.Key)
		}
	}
	return out
}

// CreatureEyes implements play.CombatRoster: what a creature notices with, from
// its stat block: the passive Perception it lists and its senses (MR-035, MR-037).
// False for a key that is not a creature of the content (SRD or the table's).
func (s *Service) CreatureEyes(ctx context.Context, tx pgx.Tx, campaignID, monsterKey string) (maplink.Eyes, bool, error) {
	content, err := s.contentFor(ctx, tx, campaignID)
	if err != nil {
		return maplink.Eyes{}, false, wrap("read rules content", err)
	}
	d, ok := content.MonsterDerived(monsterKey)
	if !ok {
		return maplink.Eyes{}, false, nil
	}
	return maplink.Eyes{Passive: d.PassivePerception, Senses: sensesOf(d.Senses)}, true, nil
}

// intelligenceOf is the Intelligence score of the creature, 0 when its page has none.
func intelligenceOf(d rules.Derived) int {
	i := slices.IndexFunc(d.Abilities, func(a rules.AbilityScore) bool { return a.Ability == rules.INT })
	if i < 0 {
		return 0
	}
	return d.Abilities[i].Score
}
