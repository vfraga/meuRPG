package play

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The effects on the characters outside a running combat: how they are read, and how the
// master ends them (RN-22).

// ListCharacterEffects implements playv1connect.LastingEffectServiceHandler.
func (s *Service) ListCharacterEffects(
	ctx context.Context,
	req *connect.Request[playv1.ListCharacterEffectsRequest],
) (*connect.Response[playv1.ListCharacterEffectsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListCharacterEffectsOfCampaign(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list the characters' effects", err)
	}
	chars, owned, err := s.effectCharacters(ctx, m, rows)
	if err != nil {
		return nil, err
	}
	content, err := s.roster.RulesContent(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read the rules", err)
	}
	names := s.namesFor(ctx, m.CampaignID)
	master := m.Role == authz.RoleMaster
	out := &playv1.ListCharacterEffectsResponse{}
	for _, r := range rows {
		name, ok := chars[r.CharacterID]
		if !ok {
			continue // reserved or gone: nobody reads it
		}
		if !master && !playerReadsCharacterEffect(r, owned[r.CharacterID]) {
			continue
		}
		out.Effects = append(out.Effects, characterEffectProto(r, name, names, content, master))
	}
	return connect.NewResponse(out), nil
}

// effectCharacters reads the names of the characters the rows are on, and which of them the
// caller plays.
func (s *Service) effectCharacters(ctx context.Context, m authz.Membership, rows []playdb.CharacterEffect) (map[string]string, map[string]bool, error) {
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		if !slices.Contains(ids, r.CharacterID) {
			ids = append(ids, r.CharacterID)
		}
	}
	chars := map[string]string{} // id -> name
	owned := map[string]bool{}
	if len(ids) == 0 {
		return chars, owned, nil
	}
	found, err := s.roster.CombatCharacters(ctx, nil, m.CampaignID, ids)
	if err != nil {
		return nil, nil, s.dbError(ctx, "read the characters", err)
	}
	for _, c := range found {
		chars[c.ID] = c.Name
		owned[c.ID] = c.Player && c.PlayerUserID == m.UserID
	}
	return chars, owned, nil
}

// playerReadsCharacterEffect says a player reads what the master leaves visible, on their own
// characters, and the effects for everyone on the others (RN-10).
func playerReadsCharacterEffect(r playdb.CharacterEffect, own bool) bool {
	return r.PlayerVisible && (own || r.Audience == "all")
}

// characterEffectProto is the effect on a character as the caller reads it.
func characterEffectProto(r playdb.CharacterEffect, name string, names func(string) string, content *rules.Content, master bool) *playv1.CharacterEffect {
	e := &playv1.CharacterEffect{
		Id: r.ID, CharacterId: r.CharacterID, CharacterName: name, SourceKey: r.SourceKey, SourceNamePt: names(r.SourceKey),
		Concentration: r.Concentration, DurationTextPt: gameTimeText(r.SecondsLeft, r.Concentration),
	}
	if r.SecondsLeft != nil {
		e.SecondsLeft = r.SecondsLeft
	}
	for _, k := range r.ConditionKeys {
		e.ConditionNamesPt = append(e.ConditionNamesPt, names(k))
	}
	mods := modifiersOf(r.Modifiers)
	if d, ok := content.CombatEffect(r.SourceKey); ok {
		e.TagsPt = d.TagsPT
	} else if d, ok := content.CombatSpellEffect(r.SourceKey); ok {
		e.TagsPt = d.TagsPT
	}
	e.TagsPt = append(append([]string{}, e.TagsPt...), modifierTagsPT(mods)...)
	e.TagsPt = append(e.TagsPt, narratedNotesPT(mods)...)
	if master {
		e.PlayerVisible, e.Audience = r.PlayerVisible, audienceProto(r.Audience)
	}
	return e
}

// gameTimeText writes a game time the way the card reads it outside a combat: "dura 1 minuto",
// "dura 54 segundos", "dura 8 horas" (a round is 6 seconds; the app has no wall clock).
func gameTimeText(seconds *int32, concentration bool) string {
	if seconds == nil {
		if concentration {
			return "Dura enquanto quem a conjurou se concentrar"
		}
		return "Dura até o mestre encerrar"
	}
	left := int(*seconds)
	var parts []string
	unit := func(n int, one, many string) {
		if n == 0 {
			return
		}
		if n == 1 {
			parts = append(parts, "1 "+one)
			return
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, many))
	}
	unit(left/3600, "hora", "horas")
	unit(left%3600/60, "minuto", "minutos")
	unit(left%60, "segundo", "segundos")
	if len(parts) == 0 {
		return "Dura menos de um segundo"
	}
	return "Dura " + strings.Join(parts, " e ")
}

// endCharacterEnd is what the transaction of endCharacterEffect leaves for after it: how many
// effects ended and the vitals to tell.
type endCharacterEnd struct {
	ended int32
	told  []*playv1.CharacterVitals
}

// endCharacterEffect is EndLastingEffect for an effect on a character outside a combat: the
// effect goes, or the whole casting does with the cast that held it.
func (s *Service) endCharacterEffect(
	ctx context.Context, m authz.Membership, msg *playv1.EndLastingEffectRequest, key string,
) (*connect.Response[playv1.EndLastingEffectResponse], error) {
	id, ok := parseID(msg.GetEffectId())
	if !ok {
		return nil, errEffectNotFound()
	}
	hash := idem.Hash(msg)
	var out endCharacterEnd
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		out = endCharacterEnd{}
		return s.endCharacterEffectTx(ctx, tx, m, msg, id, key, hash, &out)
	})
	if err != nil {
		return nil, s.dbError(ctx, "end an effect", err)
	}
	s.publishCastsChanged(m.CampaignID, false)
	s.publishVitalsOf(m.CampaignID, out.told)
	return connect.NewResponse(&playv1.EndLastingEffectResponse{Ended: out.ended}), nil
}

// endCharacterEffectTx is the transaction of endCharacterEffect: lock the open session, answer a
// retry from the event it wrote, end the effect or its casting, and write the event.
func (s *Service) endCharacterEffectTx(ctx context.Context, tx pgx.Tx, m authz.Membership, msg *playv1.EndLastingEffectRequest, id, key string, hash *string, out *endCharacterEnd) error {
	q := s.queries.WithTx(tx)
	session, err := lockOpenSession(ctx, q, m.CampaignID)
	if err != nil {
		return err
	}
	done, again, err := eventByKey(ctx, q, session.ID, key, eventLastingEnded, hash)
	if err != nil {
		return err
	}
	if again {
		var ev gameTimeEvent
		_ = json.Unmarshal(done.Payload, &ev)
		out.ended = ev.Ended
		return nil
	}
	row, err := q.GetCharacterEffect(ctx, playdb.GetCharacterEffectParams{ID: id, CampaignID: m.CampaignID})
	if errors.Is(err, pgx.ErrNoRows) {
		return errEffectNotFound()
	}
	if err != nil {
		return fmt.Errorf("find the effect: %w", err)
	}
	c, err := s.openTx(ctx, combatTx{tx: tx, q: q, session: session, now: s.now(), kind: eventLastingEnded, actorUserID: m.UserID, svc: s, master: true})
	if err != nil {
		return err
	}
	if msg.GetScope() == playv1.EffectEndScope_EFFECT_END_SCOPE_CONCENTRATION_GROUP {
		out.ended, err = s.endCharacterCasting(ctx, c, row)
	} else {
		out.ended, err = s.endOneCharacterEffect(ctx, c, row)
	}
	if err != nil {
		return err
	}
	out.told = c.told
	return s.appendKeyedEvent(ctx, q, keyedEvent{sessionID: session.ID, kind: eventLastingEnded, userID: m.UserID, key: key, hash: hash}, gameTimeEvent{Ended: out.ended})
}

// endCharacterCasting ends every effect of a casting on the characters, and the cast that held
// them; it answers how many ended.
func (s *Service) endCharacterCasting(ctx context.Context, c *combatTx, row playdb.CharacterEffect) (int32, error) {
	gone, err := c.q.DeleteCharacterEffectsOfGroup(ctx, playdb.DeleteCharacterEffectsOfGroupParams{CampaignID: c.session.CampaignID, GroupID: row.GroupID})
	if err != nil {
		return 0, fmt.Errorf("end the casting: %w", err)
	}
	ended := int32(len(gone)) //nolint:gosec // a handful of effects
	if err := s.afterCharacterEffectsGone(ctx, c, gone); err != nil {
		return 0, err
	}
	var touched []string
	for _, g := range gone {
		touched = append(touched, g.CharacterID)
	}
	if err := s.syncArmorBase(ctx, c, touched...); err != nil {
		return 0, err
	}
	if cast, err := c.q.GetSpellCast(ctx, playdb.GetSpellCastParams{ID: row.GroupID, CampaignID: c.session.CampaignID}); err == nil && cast.Status == castActive {
		if _, _, _, err := s.closeCast(ctx, c, cast, castEnded, endDismissed); err != nil {
			return 0, err
		}
	}
	return ended, nil
}

// endOneCharacterEffect ends one effect on a character; it answers how many ended (one).
func (s *Service) endOneCharacterEffect(ctx context.Context, c *combatTx, row playdb.CharacterEffect) (int32, error) {
	if err := c.q.DeleteCharacterEffect(ctx, row.ID); err != nil {
		return 0, fmt.Errorf("end the effect: %w", err)
	}
	if err := s.afterCharacterEffectsGone(ctx, c, []playdb.CharacterEffect{row}); err != nil {
		return 0, err
	}
	if err := s.syncArmorBase(ctx, c, row.CharacterID); err != nil {
		return 0, err
	}
	return 1, nil
}
