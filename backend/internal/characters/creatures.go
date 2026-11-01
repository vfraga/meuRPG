package characters

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"connectrpc.com/connect"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The SRD creatures (MR-037) are public rules, like the catalog and the spell
// details, so there is nothing to read from the database: these two methods
// check access and turn package rules' answers into messages.

const (
	defaultCreaturesPage = 50
	maxCreaturesPage     = 400 // the bestiary shows the whole list: 334 creatures
	maxCreatureQuery     = 100
)

var errUnknownCreature = errors.New("creature not found")

// ListCreatures implements rulesv1connect.ContentServiceHandler.
func (s *Service) ListCreatures(
	ctx context.Context,
	req *connect.Request[rulesv1.ListCreaturesRequest],
) (*connect.Response[rulesv1.ListCreaturesResponse], error) {
	// Active members only: a pending member needs no creature (RN-15).
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	page, err := readCreaturesPage(req.Msg)
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, req.Msg.GetCampaignId())
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	list, err := content.ListCreatures(page.filter)
	if err != nil {
		return nil, s.creatureListError(ctx, err)
	}
	if !isMaster(m) {
		// A table creature is the master's, whatever its state (RN-23): the list is a choice, and a player is
		// offered none. A creature a character of the caller holds is read by its key, and it is in the list.
		if list, err = s.withoutUnheldTableCreatures(ctx, m, list); err != nil {
			return nil, s.dbError(ctx, "read the caller's creatures", err)
		}
	}
	res, err := page.of(list, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(res), nil
}

// creaturesPage is the page of the bestiary a request asks for.
type creaturesPage struct {
	filter       rules.CreatureFilter
	offset, size int
}

// readCreaturesPage checks a ListCreatures request in the order the client is
// told about it: the query, the page size, the size filter, the page token.
func readCreaturesPage(msg *rulesv1.ListCreaturesRequest) (creaturesPage, error) {
	if utf8.RuneCountInString(msg.GetQuery()) > maxCreatureQuery {
		return creaturesPage{}, invalidArgument(fieldErr("query", "must have at most %d characters", maxCreatureQuery))
	}
	size := msg.GetPageSize()
	switch {
	case size == 0:
		size = defaultCreaturesPage
	case size < 0 || size > maxCreaturesPage:
		return creaturesPage{}, invalidArgument(fieldErr("page_size", "must be 1 to %d", maxCreaturesPage))
	}
	sizeWord, ok := creatureSizeWord(msg.GetSize())
	if !ok {
		return creaturesPage{}, invalidArgument(fieldErr("size", "must be a creature size"))
	}
	offset := 0
	if token := msg.GetPageToken(); token != "" {
		if offset, ok = parseCreaturesToken(token, creaturesFilterID(msg)); !ok {
			return creaturesPage{}, errNotACreaturesToken()
		}
	}
	return creaturesPage{
		filter: rules.CreatureFilter{
			Query: msg.GetQuery(), Type: msg.GetType(), Size: sizeWord, MinCR: msg.GetMinCr(), MaxCR: msg.GetMaxCr(),
			NoFly: msg.GetNoFly(), NoSwim: msg.GetNoSwim(),
		},
		offset: offset, size: int(size),
	}, nil
}

func errNotACreaturesToken() error {
	return invalidArgument(fieldErr("page_token", "is not a token of this list"))
}

// creatureListError is the answer to a filter the rules refuse, naming its
// field, or to any other failure of the list.
func (s *Service) creatureListError(ctx context.Context, err error) error {
	fe, ok := errors.AsType[*rules.CreatureFilterError](err)
	if !ok {
		return s.dbError(ctx, "list creatures", err)
	}
	switch {
	case errors.Is(fe, rules.ErrChallengeRange):
		return invalidArgument(fieldErr(fe.Field, "must not be above max_cr"))
	case errors.Is(fe, rules.ErrChallengeRating):
		return invalidArgument(fieldErr(fe.Field, "is not one of the SRD's challenge ratings"))
	}
	return invalidArgument(fieldErr(fe.Field, "is not valid"))
}

// of cuts the page out of the whole list, with the token of the next one when
// there is more.
func (p creaturesPage) of(list []rules.CreatureEntry, msg *rulesv1.ListCreaturesRequest) (*rulesv1.ListCreaturesResponse, error) {
	res := &rulesv1.ListCreaturesResponse{Total: i32(len(list))}
	if p.offset > len(list) {
		return nil, errNotACreaturesToken()
	}
	end := min(p.offset+p.size, len(list))
	for _, e := range list[p.offset:end] {
		res.Creatures = append(res.Creatures, creatureSummaryToProto(e))
	}
	if end < len(list) {
		res.NextPageToken = creaturesToken(end, creaturesFilterID(msg))
	}
	return res, nil
}

// GetCreature implements rulesv1connect.ContentServiceHandler.
func (s *Service) GetCreature(
	ctx context.Context,
	req *connect.Request[rulesv1.GetCreatureRequest],
) (*connect.Response[rulesv1.GetCreatureResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, req.Msg.GetCampaignId())
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	c, ok := content.CreatureByKey(req.Msg.GetKey())
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errUnknownCreature)
	}
	// A table creature answers a player as an unknown creature (RN-23), unless a character of the caller
	// holds one: it resolves for its owner's player, whatever its state.
	if rules.IsTableKey(req.Msg.GetKey()) && !isMaster(m) {
		holds, err := s.callerHoldsCreature(ctx, m, req.Msg.GetKey())
		if err != nil {
			return nil, s.dbError(ctx, "read the caller's creatures", err)
		}
		if !holds {
			return nil, connect.NewError(connect.CodeNotFound, errUnknownCreature)
		}
	}
	res := creatureToProto(c)
	// The attacks "Criar NPC" would copy: the very sheet the creation builds, so they never differ.
	if basic, err := npcSheetFromCreature(content, req.Msg.GetKey()); err == nil {
		for _, a := range basic.GetAttacks() {
			res.NpcAttackNames = append(res.NpcAttackNames, a.GetName())
		}
	}
	return connect.NewResponse(&rulesv1.GetCreatureResponse{Creature: res}), nil
}

// callerHoldsCreature says whether a character of the caller's holds a live creature of the key.
func (s *Service) callerHoldsCreature(ctx context.Context, m authz.Membership, key string) (bool, error) {
	held, err := callerHeldCreatures(ctx, s.queries, m)
	return held[key], err
}

// withoutUnheldTableCreatures is the list without the table creatures the caller holds none of.
func (s *Service) withoutUnheldTableCreatures(ctx context.Context, m authz.Membership, list []rules.CreatureEntry) ([]rules.CreatureEntry, error) {
	if !slices.ContainsFunc(list, func(e rules.CreatureEntry) bool { return rules.IsTableKey(e.Key) }) {
		return list, nil // nothing to hide: no query
	}
	held, err := callerHeldCreatures(ctx, s.queries, m)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(list, func(e rules.CreatureEntry) bool { return rules.IsTableKey(e.Key) && !held[e.Key] }), nil
}

// callerHeldCreatures is the set of keys of the live creatures the caller's characters hold, in two
// queries however many there are.
func callerHeldCreatures(ctx context.Context, q *charactersdb.Queries, m authz.Membership) (map[string]bool, error) {
	sheets, err := q.ListCampaignSheets(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	var mine []string
	for _, row := range sheets {
		if conv.OrZero(row.PlayerUserID) == m.UserID {
			mine = append(mine, row.ID)
		}
	}
	out := map[string]bool{}
	if len(mine) == 0 {
		return out, nil
	}
	held, err := q.ListLiveCreaturesOfCharacters(ctx, charactersdb.ListLiveCreaturesOfCharactersParams{CampaignID: m.CampaignID, CharacterIds: mine})
	if err != nil {
		return nil, err
	}
	for _, r := range held {
		out[r.CharacterCreature.MonsterKey] = true
	}
	return out, nil
}

// creaturesFilterID is a short fingerprint of the filters, so a page token only
// works for the list it came from.
func creaturesFilterID(r *rulesv1.ListCreaturesRequest) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		r.GetQuery(), r.GetType(), r.GetMaxCr(), r.GetMinCr(), r.GetSize().String(), strconv.FormatBool(r.GetNoFly()), strconv.FormatBool(r.GetNoSwim()),
	}, "\x00")))
	return hex.EncodeToString(sum[:6])
}

// The page token is the offset of the next row and the fingerprint of the
// filters, written as base64 text so a client treats it as opaque.
func creaturesToken(offset int, filters string) string {
	return base64.RawURLEncoding.EncodeToString([]byte("c" + strconv.Itoa(offset) + ":" + filters))
}

func parseCreaturesToken(token, filters string) (int, bool) {
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, false
	}
	rest, found := strings.CutPrefix(string(b), "c")
	n, got, hasFilters := strings.Cut(rest, ":")
	offset, err := strconv.Atoi(n)
	return offset, found && hasFilters && got == filters && err == nil && offset >= 0
}

// creatureSizeWord is the SRD's size word ("Large") of a size filter, "" for
// UNSPECIFIED (any size); ok is false for a number that is no size.
func creatureSizeWord(s rulesv1.CreatureSize) (string, bool) {
	if s == rulesv1.CreatureSize_CREATURE_SIZE_UNSPECIFIED {
		return "", true
	}
	name, ok := rulesv1.CreatureSize_name[int32(s)]
	if !ok {
		return "", false
	}
	word := strings.TrimPrefix(name, "CREATURE_SIZE_")
	return word[:1] + strings.ToLower(word[1:]), true
}

func creatureSummaryToProto(e rules.CreatureEntry) *rulesv1.CreatureSummary {
	return &rulesv1.CreatureSummary{
		Key: e.Key, Name: e.Name, NamePt: e.NamePT, Size: e.Size, SizePt: e.SizeNamePT,
		Type: e.Type, TypePt: e.TypeNamePT, Subtype: e.Subtype,
		ChallengeRating: e.ChallengeRating, Xp: i32(e.XP), CanFly: e.CanFly, CanSwim: e.CanSwim,
		ArmorClass: i32(e.ArmorClass), HitPoints: i32(e.HitPoints),
	}
}

func namedKeysToProto(list []rules.NamedKey) []*rulesv1.CreatureNamedKey {
	var out []*rulesv1.CreatureNamedKey
	for _, k := range list {
		out = append(out, &rulesv1.CreatureNamedKey{Key: k.Key, NamePt: k.NamePT})
	}
	return out
}

func damageModsToProto(list []rules.CreatureDamageMod) []*rulesv1.CreatureDamageModifier {
	var out []*rulesv1.CreatureDamageModifier
	for _, d := range list {
		out = append(out, &rulesv1.CreatureDamageModifier{Types: namedKeysToProto(d.Types), Note: d.Note})
	}
	return out
}

func traitsToProto(list []rules.CreatureAbility) []*rulesv1.CreatureTrait {
	var out []*rulesv1.CreatureTrait
	for _, a := range list {
		out = append(out, &rulesv1.CreatureTrait{Name: a.Name, Text: a.Text, Usage: a.Usage})
	}
	return out
}

func creatureToProto(c rules.Creature) *rulesv1.Creature {
	out := &rulesv1.Creature{
		Summary: creatureSummaryToProto(c.CreatureEntry), Alignment: c.Alignment,
		ArmorClass: i32(c.ArmorClass), ArmorClassLabelPt: c.ArmorClassNamePT, ArmorClassNote: c.ArmorClassNote,
		HitPoints: i32(c.HitPoints), HitDice: c.HitDice, HitPointsRoll: c.HitPointsRoll,
		SpeedWalkFt: i32(c.SpeedWalkFt), SpeedFlyFt: i32(c.SpeedFlyFt), SpeedSwimFt: i32(c.SpeedSwimFt),
		SpeedClimbFt: i32(c.SpeedClimbFt), SpeedBurrowFt: i32(c.SpeedBurrowFt), Hover: c.Hover,
		Vulnerabilities: damageModsToProto(c.Vulnerabilities), Resistances: damageModsToProto(c.Resistances),
		Immunities:          damageModsToProto(c.Immunities),
		ConditionImmunities: namedKeysToProto(c.ConditionImmunities),
		PassivePerception:   i32(c.PassivePerception), Languages: c.Languages, ProficiencyBonus: i32(c.ProficiencyBonus),
		Traits: traitsToProto(c.Traits), Reactions: traitsToProto(c.Reactions), LegendaryActions: traitsToProto(c.LegendaryActions),
	}
	for _, a := range c.Abilities {
		out.Abilities = append(out.Abilities, &rulesv1.CreatureAbilityScore{
			Ability: abilityToProto[a.Ability], NamePt: a.NamePT, Score: i32(a.Score), Modifier: i32(a.Modifier),
		})
	}
	for _, s := range c.Saves {
		out.SavingThrows = append(out.SavingThrows, &rulesv1.CreatureBonus{Ability: abilityToProto[s.Ability], NamePt: s.NamePT, Bonus: i32(s.Bonus)})
	}
	for _, s := range c.Skills {
		out.Skills = append(out.Skills, &rulesv1.CreatureBonus{Ability: abilityToProto[s.Ability], Key: s.Key, NamePt: s.NamePT, Bonus: i32(s.Bonus)})
	}
	for _, s := range c.Senses {
		out.Senses = append(out.Senses, &rulesv1.Sense{Key: s.Source, NamePt: s.NamePT, RangeFt: i32(s.RangeFt), Sense: s.Key})
	}
	for _, a := range c.Actions {
		out.Actions = append(out.Actions, creatureActionToProto(a))
	}
	return out
}

var (
	creatureActionKindToProto = map[string]rulesv1.CreatureActionKind{
		rules.ActionKindAttack:      rulesv1.CreatureActionKind_CREATURE_ACTION_KIND_ATTACK,
		rules.ActionKindSave:        rulesv1.CreatureActionKind_CREATURE_ACTION_KIND_SAVE,
		rules.ActionKindMultiattack: rulesv1.CreatureActionKind_CREATURE_ACTION_KIND_MULTIATTACK,
		rules.ActionKindOther:       rulesv1.CreatureActionKind_CREATURE_ACTION_KIND_OTHER,
	}
	creatureActionClassToProto = map[string]rulesv1.CreatureActionClass{
		rules.ActionStructured: rulesv1.CreatureActionClass_CREATURE_ACTION_CLASS_STRUCTURED,
		rules.ActionPartial:    rulesv1.CreatureActionClass_CREATURE_ACTION_CLASS_PARTIAL,
		rules.ActionText:       rulesv1.CreatureActionClass_CREATURE_ACTION_CLASS_TEXT,
	}
	creatureAreaShapeToProto = map[string]rulesv1.CreatureAreaShape{
		rules.AreaCone:     rulesv1.CreatureAreaShape_CREATURE_AREA_SHAPE_CONE,
		rules.AreaLine:     rulesv1.CreatureAreaShape_CREATURE_AREA_SHAPE_LINE,
		rules.AreaSphere:   rulesv1.CreatureAreaShape_CREATURE_AREA_SHAPE_SPHERE,
		rules.AreaCube:     rulesv1.CreatureAreaShape_CREATURE_AREA_SHAPE_CUBE,
		rules.AreaCylinder: rulesv1.CreatureAreaShape_CREATURE_AREA_SHAPE_CYLINDER,
		rules.AreaRadius:   rulesv1.CreatureAreaShape_CREATURE_AREA_SHAPE_RADIUS,
	}
)

// creatureActionToProto is an action of a stat block with the plan the server
// reads from it.
func creatureActionToProto(a rules.CreatureAction) *rulesv1.CreatureAction {
	act := &rulesv1.CreatureAction{Name: a.Name, NamePt: a.NamePT, Text: a.Text, Usage: a.Usage, HasAttack: a.HasAttack, AttackBonus: i32(a.AttackBonus)}
	for _, d := range a.Damage {
		act.Damage = append(act.Damage, &rulesv1.CreatureDamagePart{Dice: d.Dice, DamageTypeKey: d.TypeKey, DamageTypePt: d.TypeNamePT})
	}
	if a.Save != nil {
		act.Save = creatureSaveToProto(a.Save)
	}
	for _, routine := range a.Multiattack {
		r := &rulesv1.CreatureMultiattackRoutine{}
		for _, x := range routine {
			r.Attacks = append(r.Attacks, &rulesv1.CreatureAttackCount{Name: x.Name, Count: i32(x.Count), Kind: x.Kind, CountText: x.Text})
		}
		act.Multiattack = append(act.Multiattack, r)
	}
	if p := a.Plan; p != nil {
		act.Key, act.Kind, act.PlanClass = p.Key, creatureActionKindToProto[p.Kind], creatureActionClassToProto[p.Class]
		if at := p.Attack; at != nil {
			act.Melee, act.SpellAttack = at.Melee, at.Spell
			act.ReachFt, act.RangeFt, act.LongRangeFt = i32(at.ReachFt), i32(at.RangeFt), i32(at.LongRangeFt)
		}
		if h := p.HitCondition; h != nil {
			act.HitCondition = &rulesv1.CreatureHitCondition{ConditionKey: h.ConditionKey, ConditionPt: h.ConditionNamePT, EscapeDc: i32(h.EscapeDC), MaxSize: h.MaxSize}
		}
		if ar := p.Area; ar != nil {
			act.Area = &rulesv1.CreatureArea{Shape: creatureAreaShapeToProto[ar.Shape], SizeFt: i32(ar.SizeFt), WidthFt: i32(ar.WidthFt)}
		}
	}
	return act
}

// creatureSaveToProto is the saving throw of an action, with what the plan
// reads of it: the damage it governs, the condition of a failure and whether it
// rides on an attack.
func creatureSaveToProto(s *rules.CreatureSave) *rulesv1.CreatureSave {
	out := &rulesv1.CreatureSave{Ability: abilityToProto[s.Ability], Dc: i32(s.DC), OnSuccess: saveSuccessToProto[s.OnSuccess]}
	if p := s.Plan; p != nil {
		for _, d := range p.Damage {
			out.Damage = append(out.Damage, &rulesv1.CreatureDamagePart{Dice: d.DiceText, DamageTypeKey: d.TypeKey, DamageTypePt: d.TypeNamePT})
		}
		out.ConditionKey, out.ConditionPt = p.ConditionKey, p.ConditionNamePT
		out.OnHit, out.Duration, out.RepeatSave = p.OnHit, p.Duration, p.RepeatSave
	}
	return out
}
