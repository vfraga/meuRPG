package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// charEffectEvent is the payload of the master's giving an effect to characters outside a
// combat: keys and ids, never a name.
type charEffectEvent struct {
	Key     string   `json:"key"`
	Targets []string `json:"targets"`
	Effects []string `json:"effects"`
}

// charEffectInput is what AddCharacterEffect reads from its request before it opens the
// transaction: the key, the characters and the duration the master picked.
type charEffectInput struct {
	key          string
	characterIDs []string
	kind         string // the duration kind; empty for the effect's own
	seconds      int32
}

// parseCharEffectInput checks the request: the key, the characters, the label and the
// duration.
func parseCharEffectInput(msg *playv1.AddCharacterEffectRequest) (charEffectInput, error) {
	var in charEffectInput
	key, err := parseKey(msg.GetIdempotencyKey())
	if err != nil {
		return in, err
	}
	in.key = key
	if in.characterIDs, err = parseCharacterIDs(msg.GetCharacterIds()); err != nil {
		return in, err
	}
	if err := checkEffectLabel(msg.GetPlayerLabel()); err != nil {
		return in, err
	}
	in.kind, in.seconds, err = charDurationOf(durationKindOf(msg.GetDurationKind()), msg.GetSeconds())
	return in, err
}

// parseCharacterIDs reads the characters: one to ten, all UUIDs, none twice.
func parseCharacterIDs(raw []string) ([]string, error) {
	if n := len(raw); n < 1 || n > maxEffectTargets {
		return nil, errInvalidEffect("character_ids must have 1 to 10 characters")
	}
	var ids []string
	for _, r := range raw {
		id, ok := parseID(r)
		if !ok {
			return nil, errInvalidEffect("character_ids must be UUIDs")
		}
		if slices.Contains(ids, id) {
			return nil, errInvalidEffect("character_ids repeats a character")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// charDurationOf is the duration kind and the seconds outside a combat: only the clock of game
// time, until dismissed and until a long rest have a meaning there.
func charDurationOf(kind string, seconds int32) (string, int32, error) {
	switch kind {
	case rules.EffectDurationRounds:
		if seconds < 1 || seconds > maxGameTimeStep {
			return kind, seconds, errInvalidEffect("seconds must be 1 to 86400")
		}
	case "", rules.EffectDurationUntilDismissed, rules.EffectDurationLongRest:
		seconds = 0
	case rules.EffectDurationConcentration:
		kind, seconds = rules.EffectDurationUntilDismissed, 0 // nobody concentrates on it: it lasts until the master ends it
	default:
		return kind, seconds, errInvalidEffect("duration_kind has no meaning outside a combat")
	}
	return kind, seconds, nil
}

// AddCharacterEffect implements playv1connect.LastingEffectServiceHandler.
func (s *Service) AddCharacterEffect(
	ctx context.Context,
	req *connect.Request[playv1.AddCharacterEffectRequest],
) (*connect.Response[playv1.AddCharacterEffectResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	in, err := parseCharEffectInput(req.Msg)
	if err != nil {
		return nil, err
	}
	hash := idem.Hash(req.Msg)
	var made charEffectEvent
	var vitals []*playv1.CharacterVitals
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		made, vitals, err = s.giveCharacterEffect(ctx, tx, m, in, hash, req.Msg)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "give an effect to characters", err)
	}
	s.publishCastsChanged(m.CampaignID, false)
	s.publishVitalsOf(m.CampaignID, vitals)
	return connect.NewResponse(&playv1.AddCharacterEffectResponse{EffectIds: made.Effects}), nil
}

// giveCharacterEffect is the transaction of AddCharacterEffect: read and validate (the open
// session, the retry, the characters), resolve what the effect is, write it and the event. It
// answers what was made and the vitals the streams are told after the commit.
func (s *Service) giveCharacterEffect(ctx context.Context, tx pgx.Tx, m authz.Membership, in charEffectInput, hash *string, msg *playv1.AddCharacterEffectRequest) (charEffectEvent, []*playv1.CharacterVitals, error) {
	q := s.queries.WithTx(tx)
	session, err := lockOpenSession(ctx, q, m.CampaignID)
	if err != nil {
		return charEffectEvent{}, nil, err
	}
	done, again, err := eventByKey(ctx, q, session.ID, in.key, eventLastingAdded, hash)
	if err != nil {
		return charEffectEvent{}, nil, err
	}
	if again {
		var made charEffectEvent
		err := json.Unmarshal(done.Payload, &made)
		return made, nil, err
	}
	if err := s.requireOutsideCombat(ctx, tx, q, m.CampaignID, session.ID, in.characterIDs); err != nil {
		return charEffectEvent{}, nil, err
	}
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now(), kind: eventLastingAdded, actorUserID: m.UserID, svc: s, master: true})
	if err != nil {
		return charEffectEvent{}, nil, err
	}
	plan, err := s.planCharEffect(ctx, c, in, msg)
	if err != nil {
		return charEffectEvent{}, nil, err
	}
	made, err := s.putCharEffects(ctx, c, m.CampaignID, in, plan)
	if err != nil {
		return charEffectEvent{}, nil, err
	}
	if err := s.syncArmorBase(ctx, c, slices.Clone(made.Targets)...); err != nil { // it sorts what it is given: the event keeps the order of the request
		return charEffectEvent{}, nil, err
	}
	if err := s.appendKeyedEvent(ctx, q, keyedEvent{sessionID: session.ID, kind: eventLastingAdded, userID: m.UserID, key: in.key, hash: hash}, made); err != nil {
		return charEffectEvent{}, nil, err
	}
	return made, c.told, nil
}

// requireOutsideCombat refuses characters the campaign's roster does not know as players' (an NPC,
// a reserved character) and those that fight in the open combat: the effect is theirs there.
func (s *Service) requireOutsideCombat(ctx context.Context, tx pgx.Tx, q *playdb.Queries, campaignID, sessionID string, characterIDs []string) error {
	found, err := s.roster.CombatCharacters(ctx, tx, campaignID, characterIDs)
	if err != nil {
		return err
	}
	for _, id := range characterIDs {
		if !slices.ContainsFunc(found, func(ch link.Character) bool { return ch.ID == id && ch.Player && !ch.Reserved }) {
			return connect.NewError(connect.CodeNotFound, errors.New("character not found"))
		}
	}
	_, cs, fighting, err := openCombatants(ctx, q, sessionID)
	if err != nil || !fighting {
		return err
	}
	if slices.ContainsFunc(cs, func(o playdb.Combatant) bool {
		return slices.Contains(characterIDs, o.CharacterID) && backedByCharacter(o)
	}) {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("the character is in a combat: add the effect there"))
	}
	return nil
}

// charEffectPlan is what the effect is, worked out from the catalog and the request, before any
// character is touched.
type charEffectPlan struct {
	catalogKey string
	def        *rules.EffectDef
	conditions []string
	sourceKind string
	kind       string // the duration kind
	seconds    *int32
	visible    bool
	audience   string // as the table keeps it: "all" or "owner"
	label      *string
	ability    string
}

// charEffectSource is where the effect comes from in the catalog, with what it lasts when the
// master says nothing.
type charEffectSource struct {
	def        *rules.EffectDef
	conditions []string
	sourceKind string
	audience   string
	fallback   string
	fallbackS  *int32
}

// planCharEffect resolves the request into the plan: the catalog entry, the duration, who reads
// it, the label and the ability.
func (s *Service) planCharEffect(ctx context.Context, c *combatTx, in charEffectInput, msg *playv1.AddCharacterEffectRequest) (charEffectPlan, error) {
	plan := charEffectPlan{catalogKey: msg.GetCatalogKey(), visible: true}
	content, err := s.contentOf(ctx, c)
	if err != nil {
		return plan, err
	}
	src, err := charEffectSourceOf(content, plan.catalogKey)
	if err != nil {
		return plan, err
	}
	plan.def, plan.conditions, plan.sourceKind = src.def, src.conditions, src.sourceKind
	if plan.kind, plan.seconds, err = charEffectClock(in, src); err != nil {
		return plan, err
	}
	audience := chosenAudience(msg.GetAudience(), src.audience)
	plan.audience = "all"
	if audience == rules.EffectVisibilityOwner {
		plan.audience = audienceOwner
	}
	if v := msg.PlayerVisible; v != nil {
		plan.visible = *v
	}
	if l := msg.GetPlayerLabel(); l != "" {
		plan.label = &l
	}
	if plan.def != nil {
		plan.ability, err = s.abilityChoiceOf(ctx, c, plan.catalogKey, msg.GetAbilityKey())
	}
	return plan, err
}

// charEffectSourceOf looks the key up in the part of the catalog it names.
func charEffectSourceOf(content *rules.Content, key string) (charEffectSource, error) {
	switch catalogKindOf(key) {
	case catalogCondition:
		return charConditionSource(content, key)
	case catalogSpell:
		return charSpellSource(content, key)
	}
	return charMasterSource(content, key)
}

// charConditionSource is a condition: until dismissed, read as the condition says.
func charConditionSource(content *rules.Content, key string) (charEffectSource, error) {
	info, err := conditionOfCatalog(content, key)
	if err != nil {
		return charEffectSource{}, err
	}
	return charEffectSource{conditions: []string{key}, sourceKind: "master", audience: info.Visibility, fallback: rules.EffectDurationUntilDismissed}, nil
}

// charSpellSource is a spell's effect: it lasts as long as the spell does, when the spell is timed.
func charSpellSource(content *rules.Content, key string) (charEffectSource, error) {
	def, err := spellEffectOfCatalog(content, key)
	if err != nil {
		return charEffectSource{}, err
	}
	src := charEffectSource{def: def, sourceKind: "spell", audience: rules.EffectVisibilityPublic, fallback: rules.EffectDurationUntilDismissed}
	if def.Visibility != "" {
		src.audience = def.Visibility
	}
	if secs, timed := content.SpellEffectSeconds(key); timed && secs > 0 {
		src.fallback, src.fallbackS = rules.EffectDurationRounds, new(clamp32(secs, 1, maxGameTimeStep))
	}
	return src, nil
}

// charMasterSource is an effect of the master's own, with the time the catalog gives it.
func charMasterSource(content *rules.Content, key string) (charEffectSource, error) {
	def, err := masterEffectOfCatalog(content, key)
	if err != nil {
		return charEffectSource{}, err
	}
	src := charEffectSource{def: def, sourceKind: "master", audience: rules.EffectVisibilityPublic, fallback: rules.EffectDurationUntilDismissed}
	if def.Visibility != "" {
		src.audience = def.Visibility
	}
	if def.Duration != nil && def.Duration.Kind == rules.EffectDurationRounds {
		src.fallback, src.fallbackS = rules.EffectDurationRounds, new(clamp32(def.Duration.Rounds*rules.SecondsPerRound, 1, maxGameTimeStep))
	}
	return src, nil
}

// charEffectClock is the kind and the seconds the effect is written with: the master's, or the
// catalog's when the master chose none.
func charEffectClock(in charEffectInput, src charEffectSource) (string, *int32, error) {
	kind, secondsLeft := in.kind, (*int32)(nil)
	seconds := in.seconds
	switch kind {
	case "":
		kind, secondsLeft = src.fallback, src.fallbackS
	case rules.EffectDurationRounds:
		secondsLeft = &seconds
	}
	if kind == rules.EffectDurationRounds && secondsLeft == nil {
		return kind, nil, errInvalidEffect("seconds is required for a timed effect")
	}
	return kind, secondsLeft, nil
}

// putCharEffects writes the effect on each character, one record for each, in the order of the
// request, and answers what it made. A character the effect takes no hold of is left out; if it
// takes no hold of any, the master is told.
func (s *Service) putCharEffects(ctx context.Context, c *combatTx, campaignID string, in charEffectInput, plan charEffectPlan) (charEffectEvent, error) {
	var made charEffectEvent
	group := uuid.New().String()
	for _, id := range in.characterIDs {
		mods, ok, err := s.charEffectModifiers(ctx, c, plan, id)
		if err != nil {
			return made, err
		}
		if !ok {
			continue // Armadura Arcana on a creature that wears armor takes no hold
		}
		rowID, err := s.insertCharEffect(ctx, c, campaignID, plan, id, group, mods)
		if err != nil {
			return made, err
		}
		made.Targets, made.Effects = append(made.Targets, id), append(made.Effects, rowID)
	}
	if len(made.Effects) == 0 {
		return made, connect.NewError(connect.CodeFailedPrecondition, errors.New("the effect takes no hold of these characters"))
	}
	made.Key = plan.catalogKey
	return made, nil
}

// charEffectModifiers are the modifiers the effect gives one character; false when it takes no
// hold of it.
func (s *Service) charEffectModifiers(ctx context.Context, c *combatTx, plan charEffectPlan, characterID string) ([]rules.EffectModifier, bool, error) {
	if plan.def == nil {
		return []rules.EffectModifier{}, true, nil
	}
	opts, err := s.castOptsOf(ctx, c, plan.def, plan.ability, "")
	if err != nil {
		return nil, false, err
	}
	return s.modifiersFor(ctx, c, plan.def, characterID, opts)
}

// insertCharEffect writes one character's record (replacing the same spell's older one) and
// answers its id.
func (s *Service) insertCharEffect(ctx context.Context, c *combatTx, campaignID string, plan charEffectPlan, characterID, group string, mods []rules.EffectModifier) (string, error) {
	body, err := json.Marshal(mods)
	if err != nil {
		return "", fmt.Errorf("encode the modifiers: %w", err)
	}
	if plan.sourceKind == "spell" {
		if _, err := c.q.DeleteCharacterEffectsOfSpell(ctx, playdb.DeleteCharacterEffectsOfSpellParams{CharacterID: characterID, SourceKey: plan.catalogKey, GroupID: group}); err != nil {
			return "", fmt.Errorf("replace the same spell: %w", err)
		}
	}
	p := playdb.InsertCharacterEffectParams{
		CampaignID: campaignID, CharacterID: characterID, GroupID: group, SourceKey: plan.catalogKey, SourceKind: plan.sourceKind,
		ConditionKeys: append([]string{}, plan.conditions...), Modifiers: body, DurationKind: plan.kind, SecondsLeft: plan.seconds,
		PlayerVisible: plan.visible, Audience: plan.audience, PlayerLabel: plan.label, CreatedAt: c.now,
	}
	if plan.def != nil {
		withCharEffectDef(&p, plan.def)
	}
	row, err := c.q.InsertCharacterEffect(ctx, p)
	if err != nil {
		return "", fmt.Errorf("put the effect on the character: %w", err)
	}
	return row.ID, nil
}

// withCharEffectDef copies what the catalog entry adds to a record: its conditions, the saving
// throws and the damage it deals.
func withCharEffectDef(p *playdb.InsertCharacterEffectParams, def *rules.EffectDef) {
	p.ConditionKeys = append([]string{}, def.Conditions...)
	if def.EndSave != nil {
		p.EndSaveAbility = &def.EndSave.Ability
	}
	if def.StartSave != nil {
		p.StartSaveAbility = &def.StartSave.Ability
		if def.StartSave.OnFail != "" {
			p.OnFailEffect = &def.StartSave.OnFail
		}
	}
	if t := def.Trigger; t != nil {
		p.TriggerDice, p.TriggerDamageType, p.TriggerMaxTriggers = &t.Dice, &t.DamageType, new(int32(t.MaxTriggers)) //nolint:gosec // a small number of the file
	}
}
