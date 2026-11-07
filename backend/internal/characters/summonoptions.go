package characters

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
)

// GetSummonOptions implements charactersv1connect.CharacterServiceHandler: what
// a character can summon from its sheet (MR-037). It is a read of the same
// rules CastSummon enforces (rules.SummonOptions for what a circle may bring,
// summonStanding for what the sheet lets the character cast), so the
// "Criaturas" panel keeps no copy of them. Same access as ListCharacterCreatures:
// the master and the owner's player; anyone else gets `not_found` (RN-20). It
// carries no hit points.
func (s *Service) GetSummonOptions(
	ctx context.Context,
	req *connect.Request[charactersv1.GetSummonOptionsRequest],
) (*connect.Response[charactersv1.GetSummonOptionsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	id, ok := uid.Canonical(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	// The character, its slots and the creatures a casting would send away are one
	// moment: a casting that commits between the reads would show the slot still
	// free next to the familiar it just made.
	var out *charactersv1.GetSummonOptionsResponse
	err = db.ReadTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		out, err = s.readSummonOptions(ctx, tx, m, id) // the closure may run again: each run starts afresh
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "read the summon options", err)
	}
	return connect.NewResponse(out), nil
}

// readSummonOptions reads the options inside the read transaction: empty for a
// character with nothing to cast.
func (s *Service) readSummonOptions(ctx context.Context, tx pgx.Tx, m authz.Membership, id string) (*charactersv1.GetSummonOptionsResponse, error) {
	out := &charactersv1.GetSummonOptionsResponse{}
	q := s.queries.WithTx(tx)
	if err := checkSummonOptionsCharacter(ctx, q, m, id); err != nil {
		return out, err
	}
	b, err := s.summonCharacter(ctx, tx, m.CampaignID, id)
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return out, nil // no full sheet (or a dead character): nothing to cast
		}
		return out, err
	}
	vitals, err := s.getVitals(ctx, tx, m.CampaignID, id, false)
	if err != nil {
		return out, err
	}
	out.Slots = summonSlots(vitals)
	content, err := s.contentFor(ctx, tx, m.CampaignID)
	if err != nil {
		return out, wrap("read rules content", err)
	}
	d := rules.Derive(b, content)
	used := usageOf(vitals)
	for _, key := range content.SummonSpells() {
		spell, err := s.summonSpellOptions(ctx, q, content, m.CampaignID, id, b, d, key, summonSpend{slots: out.Slots, used: used})
		if err != nil {
			return out, err
		}
		if spell != nil {
			out.Spells = append(out.Spells, spell)
		}
	}
	return out, nil
}

// checkSummonOptionsCharacter is the access of ListCharacterCreatures: a
// player's character the caller may see.
func checkSummonOptionsCharacter(ctx context.Context, q *charactersdb.Queries, m authz.Membership, id string) error {
	row, err := q.GetCharacter(ctx, charactersdb.GetCharacterParams{CampaignID: m.CampaignID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return errCharacterNotFound()
	}
	if err != nil {
		return wrap("read a character", err)
	}
	if !canSee(m, row.Kind, row.Status, row.PlayerUserID) {
		return errCharacterNotFound()
	}
	if row.Kind != kindPlayer {
		return invalidArgument(fieldErr("character_id", "is an NPC: only a player's character has creatures"))
	}
	return nil
}

// summonSlots are the character's slots, lowest circle first, the pact magic
// slots after their circle.
func summonSlots(vitals *playv1.CharacterVitals) []*charactersv1.SummonSlot {
	var out []*charactersv1.SummonSlot
	for _, u := range vitals.GetSpellSlots() {
		out = append(out, &charactersv1.SummonSlot{Level: u.GetLevel(), Total: u.GetTotal(), Free: u.GetTotal() - u.GetUsed()})
	}
	if p := vitals.GetPactSlots(); p != nil && p.GetTotal() > 0 {
		out = append(out, &charactersv1.SummonSlot{Level: p.GetSlotLevel(), Pact: true, Total: p.GetTotal(), Free: p.GetTotal() - p.GetUsed()})
		slices.SortStableFunc(out, func(a, c *charactersv1.SummonSlot) int { return int(a.GetLevel() - c.GetLevel()) })
	}
	return out
}

// summonSpend is what the character has to cast with: its slots and the uses it has spent of its resources.
type summonSpend struct {
	slots []*charactersv1.SummonSlot
	used  combat.Usage
}

// summonSpellOptions is one spell's entry, or nil when the character cannot cast
// it (no ritual and no ready slot cast).
func (s *Service) summonSpellOptions(ctx context.Context, q *charactersdb.Queries, content *rules.Content, campaignID, characterID string, b rules.Build, d rules.Derived, key string, spend summonSpend) (*charactersv1.SummonSpellOptions, error) {
	det, ok := content.SpellDetails(key)
	if !ok {
		return nil, nil
	}
	slots := spend.slots
	// A feature's summon: its options also need the rule (the resource it spends).
	if rule, table := content.SummonRule(key); table && isFeatureSummon(content, key) {
		return s.featureSummonOptions(ctx, q, content, campaignID, characterID, b, d, det, rule, spend.used)
	}
	own, err := content.SummonOptions(key, det.Spell.Level, b)
	if err != nil {
		return nil, fmt.Errorf("summon options of %s: %w", key, err)
	}
	st := s.summonStanding(d, key, own.Ritual)
	canSlot := st.known && st.prepared
	if !st.canRitual && !canSlot {
		return nil, nil
	}
	out := &charactersv1.SummonSpellOptions{
		SpellKey: key, NamePt: content.NamePT(key), Level: int32(det.Spell.Level), CastingTimePt: castingTimePT(own.CastingTime), //nolint:gosec // a spell's circle is 1 to 9
		Ritual: own.Ritual, Concentration: own.Concentration, CanRitual: st.canRitual, CanCastWithSlot: canSlot,
	}
	for _, c := range summonCircles(det.Spell.Level, st.canRitual, canSlot, slots) {
		if sc, ok := summonCircleOptions(content, key, c, b); ok {
			out.Circles = append(out.Circles, sc)
		}
	}
	if out.Replaces, err = summonReplaces(ctx, q, content, campaignID, characterID, key, own.Concentration); err != nil {
		return nil, err
	}
	return out, nil
}

// featureSummonOptions is the entry of a feature's summon: nil when the character does not have the feature (or not
// yet), else the one circle the summon is cast at, the use it spends and the uses left. With no use left the entry
// stays, with no circle to cast at.
func (s *Service) featureSummonOptions(ctx context.Context, q *charactersdb.Queries, content *rules.Content, campaignID, characterID string, b rules.Build, d rules.Derived, det *rules.SpellDetails, rule rules.SummonRule, used combat.Usage) (*charactersv1.SummonSpellOptions, error) {
	key := det.Spell.Key
	i := slices.IndexFunc(d.GrantedSpells, func(g rules.GrantedSpell) bool { return g.Spell.Key == key && g.Source == rule.Feature })
	if i < 0 {
		return nil, nil
	}
	grant := combat.GrantOf(d, used, d.GrantedSpells[i])
	own, err := content.SummonOptions(key, det.Spell.Level, b)
	if err != nil {
		return nil, fmt.Errorf("summon options of %s: %w", key, err)
	}
	out := &charactersv1.SummonSpellOptions{
		SpellKey: key, NamePt: content.NamePT(key), Level: i32(det.Spell.Level), CastingTimePt: castingTimePT(own.CastingTime),
		Concentration: own.Concentration, GrantedBy: rule.Feature, ResourceKey: grant.Resource, UsesLeft: i32(grant.UsesLeft),
	}
	if class, level, ok := content.ClassFeatureSource(rule.Feature); ok {
		out.GrantedByLabelPt = fmt.Sprintf("%s %d", content.NamePT(class), level)
	}
	if grant.UsesLeft > 0 {
		if sc, ok := summonCircleOptions(content, key, det.Spell.Level, b); ok {
			out.Circles = append(out.Circles, sc)
		}
	}
	if out.Replaces, err = summonReplaces(ctx, q, content, campaignID, characterID, key, own.Concentration); err != nil {
		return nil, err
	}
	return out, nil
}

// summonCircles are the circles the spell can be cast at, lowest first: its own
// as a ritual, and with a slot each circle of a slot from the spell's up.
func summonCircles(level int, canRitual, canSlot bool, slots []*charactersv1.SummonSlot) []int {
	circles := map[int]bool{}
	if canRitual {
		circles[level] = true
	}
	if canSlot {
		for _, sl := range slots {
			if int(sl.GetLevel()) >= level && sl.GetTotal() > 0 {
				circles[int(sl.GetLevel())] = true
			}
		}
	}
	levels := make([]int, 0, len(circles))
	for c := range circles {
		levels = append(levels, c)
	}
	slices.Sort(levels)
	return levels
}

// summonCircleOptions is what a casting at the circle may bring; false for a
// circle the spell does not take (a pact slot above the spell's reach).
func summonCircleOptions(content *rules.Content, key string, circle int, b rules.Build) (*charactersv1.SummonCircle, bool) {
	choices, err := content.SummonOptions(key, circle, b)
	if err != nil {
		return nil, false
	}
	sc := &charactersv1.SummonCircle{Circle: int32(circle)} //nolint:gosec // 1 to 9
	for _, o := range choices.Options {
		opt := &charactersv1.SummonOption{Count: int32(o.Count), Type: o.Type, MaxCr: o.MaxCR, Attack: creatureAttackNumber[o.Attack]} //nolint:gosec // a few dozen at most
		for _, f := range o.Forms {
			opt.Forms = append(opt.Forms, &charactersv1.SummonForm{MonsterKey: f.Key, NamePt: content.NamePT(f.Key), Attack: creatureAttackNumber[f.Attack]})
		}
		sc.Options = append(sc.Options, opt)
	}
	return sc, true
}

// summonReplaces is what a casting sends away: a new familiar replaces the old
// one, a table spell may replace the creatures of its own old casting, and a new
// concentration ends the creatures of the old one (SummonCreatures).
func summonReplaces(ctx context.Context, q *charactersdb.Queries, content *rules.Content, campaignID, characterID, key string, concentration bool) ([]*charactersv1.ReplacedCreature, error) {
	var out []*charactersv1.ReplacedCreature
	if rule, table := content.SummonRule(key); table && rule.ReplacesSame {
		rows, err := q.ListLiveCreaturesOfSummon(ctx, charactersdb.ListLiveCreaturesOfSummonParams{CampaignID: campaignID, CharacterID: characterID, SummonKey: key})
		if err != nil {
			return nil, wrap("list the creatures of the spell", err)
		}
		out = appendReplaced(out, content, rows, func(r charactersdb.ListLiveCreaturesOfSummonRow) charactersdb.CharacterCreature {
			return r.CharacterCreature
		})
	}
	if key == "spell:find-familiar" {
		rows, err := q.ListLiveFamiliars(ctx, charactersdb.ListLiveFamiliarsParams{CampaignID: campaignID, CharacterID: characterID})
		if err != nil {
			return nil, wrap("list the familiars", err)
		}
		out = appendReplaced(out, content, rows, func(r charactersdb.ListLiveFamiliarsRow) charactersdb.CharacterCreature { return r.CharacterCreature })
	}
	if concentration {
		rows, err := q.ListLiveCreaturesOnConcentration(ctx, charactersdb.ListLiveCreaturesOnConcentrationParams{CampaignID: campaignID, CharacterID: characterID})
		if err != nil {
			return nil, wrap("list the creatures that depend on a concentration", err)
		}
		out = appendReplaced(out, content, rows, func(r charactersdb.ListLiveCreaturesOnConcentrationRow) charactersdb.CharacterCreature {
			return r.CharacterCreature
		})
	}
	return out, nil
}

// appendReplaced adds the creatures of the rows to out, each once: a creature of the same table spell can be on
// the concentration too.
func appendReplaced[T any](out []*charactersv1.ReplacedCreature, content *rules.Content, rows []T, pick func(T) charactersdb.CharacterCreature) []*charactersv1.ReplacedCreature {
	for _, r := range rows {
		c := pick(r)
		if !slices.ContainsFunc(out, func(o *charactersv1.ReplacedCreature) bool { return o.GetId() == c.ID }) {
			out = append(out, replaced(content, c))
		}
	}
	return out
}

func replaced(content *rules.Content, c charactersdb.CharacterCreature) *charactersv1.ReplacedCreature {
	return &charactersv1.ReplacedCreature{Id: c.ID, Name: c.Name, MonsterKey: c.MonsterKey, MonsterNamePt: content.NamePT(c.MonsterKey)}
}

// castingTimePT says a summoning spell's casting time in Portuguese: "1 hora",
// "10 minutos", "1 ação", "1 reação".
func castingTimePT(t rules.CastingTime) string {
	n := max(t.Amount, 1)
	word := ""
	switch t.Unit {
	case rules.CastHour:
		word = map[bool]string{true: "hora", false: "horas"}[n == 1]
	case rules.CastMinute:
		word = map[bool]string{true: "minuto", false: "minutos"}[n == 1]
	case rules.CastAction:
		word = map[bool]string{true: "ação", false: "ações"}[n == 1]
	case rules.CastBonusAction:
		word = map[bool]string{true: "ação bônus", false: "ações bônus"}[n == 1]
	case rules.CastReaction:
		word = map[bool]string{true: "reação", false: "reações"}[n == 1]
	default:
		return t.Raw
	}
	return fmt.Sprintf("%d %s", n, word)
}
