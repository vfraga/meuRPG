package play

import (
	"context"
	"errors"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/live"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// What the casts outside a combat show each person (RN-10, RN-20): the master reads
// every cast; a player reads the casts of the characters they may see, never one an NPC
// that is not on the stage cast, and the hit points, healing and armor class of a target
// only when the target is their own character.

// castLens is who is looking at the casts, and the characters they name.
type castLens struct {
	master bool
	userID string
	chars  map[string]link.Character
	name   func(key string) string
	stage  stageMap
	// hidden are the NPCs on the stage the master hid from the players: a player reads no cast of
	// theirs, and no cast names them (RN-10).
	hidden map[string]bool
}

// lensFor reads the characters and the spell names the rows name.
func (s *Service) lensFor(ctx context.Context, m authz.Membership, rows []playdb.SpellCast) (castLens, error) {
	l := castLens{master: m.Role == authz.RoleMaster, userID: m.UserID, chars: map[string]link.Character{}}
	var ids []string
	for _, r := range rows {
		ids = append(ids, r.CasterID)
		for _, t := range castTargetsOf(r) {
			ids = append(ids, t.ID)
		}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) > 0 {
		chars, err := s.roster.SessionCharacters(ctx, nil, m.CampaignID, ids)
		if err != nil {
			return l, err
		}
		for _, ch := range chars {
			l.chars[ch.ID] = ch
		}
	}
	session, err := s.queries.GetOpenGameSession(ctx, m.CampaignID)
	if err == nil {
		if l.stage, err = stageMapOf(ctx, s.queries, session.ID, stageViewer(m)); err != nil {
			return l, err
		}
		if l.hidden, err = hiddenNPCsOf(ctx, s.queries, session.ID); err != nil {
			return l, err
		}
	}
	names, err := s.roster.ContentNames(ctx, nil, m.CampaignID)
	if err != nil {
		return l, err
	}
	l.name = names
	return l, nil
}

// idOf is the ID the viewer names a character by: its own for the master and for a player
// character, and for an NPC the place it holds on the stage (a player never gets an NPC's
// character ID, RN-20; one that left the stage has none).
func (l castLens) idOf(characterID string) string {
	ch, ok := l.chars[characterID]
	if l.master || (ok && ch.Player) {
		return characterID
	}
	return l.stage.byChar[characterID]
}

// sees says the viewer may read the cast at all: a player never reads a secret cast, nor one by an
// NPC the master hid on the stage.
func (l castLens) sees(r playdb.SpellCast) bool {
	return l.master || (!r.Secret && !l.hidden[r.CasterID])
}

// mine says the viewer's player plays the character.
func (l castLens) mine(characterID string) bool {
	ch, ok := l.chars[characterID]
	return ok && ch.Player && ch.PlayerUserID != "" && ch.PlayerUserID == l.userID
}

var castStatusToProto = map[string]playv1.OutsideCastStatus{
	castCasting: playv1.OutsideCastStatus_OUTSIDE_CAST_STATUS_CASTING,
	castActive:  playv1.OutsideCastStatus_OUTSIDE_CAST_STATUS_ACTIVE,
	castEnded:   playv1.OutsideCastStatus_OUTSIDE_CAST_STATUS_ENDED,
	castFailed:  playv1.OutsideCastStatus_OUTSIDE_CAST_STATUS_FAILED,
}

var castEndToProto = map[string]playv1.OutsideCastEnd{
	endInstant:       playv1.OutsideCastEnd_OUTSIDE_CAST_END_INSTANT,
	endDismissed:     playv1.OutsideCastEnd_OUTSIDE_CAST_END_DISMISSED,
	endConcentration: playv1.OutsideCastEnd_OUTSIDE_CAST_END_CONCENTRATION,
	endRest:          playv1.OutsideCastEnd_OUTSIDE_CAST_END_REST,
	endInterrupted:   playv1.OutsideCastEnd_OUTSIDE_CAST_END_INTERRUPTED,
	endCasterGone:    playv1.OutsideCastEnd_OUTSIDE_CAST_END_CASTER_GONE,
}

var castEffectToProto = map[string]playv1.CastEffect{
	castNarrated: playv1.CastEffect_CAST_EFFECT_NARRATED,
	castHeal:     playv1.CastEffect_CAST_EFFECT_HEAL,
	castTempHP:   playv1.CastEffect_CAST_EFFECT_TEMPORARY_HIT_POINTS,
	castMaxHP:    playv1.CastEffect_CAST_EFFECT_MAX_HIT_POINTS,
	castArmor:    playv1.CastEffect_CAST_EFFECT_ARMOR_CLASS,
}

var restToProto = map[string]playv1.RestThatEnds{
	"short": playv1.RestThatEnds_REST_THAT_ENDS_SHORT,
	"long":  playv1.RestThatEnds_REST_THAT_ENDS_LONG,
}

// view is the cast as the viewer reads it.
func (l castLens) view(r playdb.SpellCast) *playv1.OutsideCast {
	caster := l.chars[r.CasterID]
	out := &playv1.OutsideCast{
		Id: r.ID, SpellKey: r.SpellKey, SpellNamePt: l.name(r.SpellKey), CasterId: l.idOf(r.CasterID), CasterName: caster.Name,
		CasterIsNpc: !caster.Player, Status: castStatusToProto[r.Status], Ritual: r.Ritual, SlotLevel: r.SlotLevel, SlotPact: r.SlotPact,
		CastingMinutes: r.CastingMinutes, Concentrating: r.Concentrating, Lasts: r.Lasts,
		StartedAt: timestamppb.New(r.StartedAt), DiceCount: r.DiceCount, DiceSides: r.DiceSides, Faces: r.RollFaces,
		RollTotal: r.RollTotal, Physical: r.Physical,
	}
	if r.DurationSeconds != nil {
		out.DurationSeconds = *r.DurationSeconds
	}
	if r.RestEnds != nil {
		out.RestEnds = restToProto[*r.RestEnds]
	}
	if r.CastAt != nil {
		out.CastAt = timestamppb.New(*r.CastAt)
	}
	if r.EndedAt != nil {
		out.EndedAt = timestamppb.New(*r.EndedAt)
	}
	if r.EndReason != nil {
		out.EndReason = castEndToProto[*r.EndReason]
	}
	if l.master {
		out.Secret = r.Secret
	}
	if l.master || l.mine(r.CasterID) {
		out.CreatureIds = r.CreatureIds
	}
	for _, t := range castTargetsOf(r) {
		if l.hidden[t.ID] && !l.master {
			continue // the cast was aimed at an NPC the master has hidden since
		}
		ch := l.chars[t.ID]
		ot := &playv1.OutsideCastTarget{CharacterId: l.idOf(t.ID), Name: ch.Name, Npc: !ch.Player}
		if l.master || l.mine(t.ID) { // RN-20: what the cast did to a target (its effect, hit points, armor class, the Life Domain's extra) is the target's own
			ot.Effect, ot.ExtraHitPoints = castEffectToProto[t.Effect], t.Extra
			ot.Amount, ot.HitPointsBefore, ot.HitPointsAfter, ot.ArmorClass = t.Amount, t.Before, t.After, t.AC
		}
		out.Targets = append(out.Targets, ot)
	}
	return out
}

// filterVitals keeps the vitals the caller may read: the master reads every one, a
// player their own character's.
func filterVitals(m authz.Membership, vitals []*playv1.CharacterVitals) []*playv1.CharacterVitals {
	if m.Role == authz.RoleMaster {
		return vitals
	}
	var out []*playv1.CharacterVitals
	for _, v := range vitals {
		if v != nil && v.GetPlayerUserId() == m.UserID {
			out = append(out, v)
		}
	}
	return out
}

// publishCastsChanged tells the streams the casts changed: the master always, the players
// unless the cast is one only the master reads.
func (s *Service) publishCastsChanged(campaignID string, secret bool) {
	s.send(campaignID, castsChangedEvent(secret))
}

// castsChangedEvent is the hint that the casts changed, for the master and, unless secret, the players.
func castsChangedEvent(secret bool) live.Event {
	aud := live.Audience{Everyone: true}
	if secret {
		aud = live.Audience{Master: true}
	}
	return live.Event{
		Audience: aud,
		Message: &playv1.WatchGameSessionResponse{Event: &playv1.WatchGameSessionResponse_SpellCastsChanged_{
			SpellCastsChanged: &playv1.WatchGameSessionResponse_SpellCastsChanged{},
		}},
	}
}

// afterCastChange tells the streams what a cast change did (after the commit) and reads
// the cast as the caller sees it. A retry of a change already made tells them too: the
// first call may have died before telling, and a hint only says "read again".
func (s *Service) afterCastChange(ctx context.Context, m authz.Membership, res combatResult, made castOutcome, _ string) (*playv1.OutsideCast, error) {
	pctx, stop := afterCommit(ctx)
	defer stop()
	ev := made.ev
	if res.repeated {
		var err error
		if ev, err = readCastEvent(res.payload); err != nil {
			return nil, s.dbError(ctx, "read the cast", err)
		}
	}
	id := ev.CastID
	if id == "" {
		id = made.row.ID
	}
	if !res.repeated {
		for _, v := range made.vitals {
			s.publishVitals(m.CampaignID, v)
		}
		if made.guard.caster.PlayerUserID != "" && (len(made.created) > 0 || len(made.dismissed) > 0) {
			s.publishCreaturesChanged(m.CampaignID, made.guard.caster.PlayerUserID)
		}
	}
	row, err := s.queries.GetSpellCast(pctx, playdb.GetSpellCastParams{ID: id, CampaignID: m.CampaignID})
	if err != nil {
		return nil, s.dbError(ctx, "read the cast", err)
	}
	if res.kind != "" || res.repeated {
		s.publishCastsChanged(m.CampaignID, row.Secret)
	}
	lens, err := s.lensFor(pctx, m, []playdb.SpellCast{row})
	if err != nil {
		return nil, s.dbError(ctx, "read the cast", err)
	}
	return lens.view(row), nil
}

// castAnswer is the answer of CastSpellOutsideCombat.
func (s *Service) castAnswer(
	ctx context.Context, m authz.Membership, res combatResult, made castOutcome,
	build func(cast *playv1.OutsideCast, vitals []*playv1.CharacterVitals, ev castEvent) *playv1.CastSpellOutsideCombatResponse,
) (*connect.Response[playv1.CastSpellOutsideCombatResponse], error) {
	cast, err := s.afterCastChange(ctx, m, res, made, "")
	if err != nil {
		return nil, err
	}
	ev := made.ev
	if res.repeated {
		ev, _ = readCastEvent(res.payload) // read once already, by afterCastChange
	}
	return connect.NewResponse(build(cast, filterVitals(m, made.vitals), ev)), nil
}

// castAnswerFinish is the answer of ConfirmCastTimePassed.
func (s *Service) castAnswerFinish(ctx context.Context, m authz.Membership, res combatResult, made castOutcome) (*connect.Response[playv1.ConfirmCastTimePassedResponse], error) {
	cast, err := s.afterCastChange(ctx, m, res, made, "")
	if err != nil {
		return nil, err
	}
	ev := made.ev
	if res.repeated {
		ev, _ = readCastEvent(res.payload)
	}
	return connect.NewResponse(&playv1.ConfirmCastTimePassedResponse{
		Cast: cast, Vitals: filterVitals(m, made.vitals), CreatureIds: ev.Created, DismissedCreatureIds: ev.Dismissed, EndedCastIds: ev.Ended,
	}), nil
}

// ListSpellCasts implements playv1connect.CastingServiceHandler.
func (s *Service) ListSpellCasts(
	ctx context.Context,
	req *connect.Request[playv1.ListSpellCastsRequest],
) (*connect.Response[playv1.ListSpellCastsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	session, err := s.openSessionOf(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	only, err := s.castsAbout(ctx, m, session.ID, req.Msg.GetCharacterId())
	if err != nil {
		return nil, err
	}
	live, err := s.queries.ListLiveSpellCasts(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list the casts", err)
	}
	logged, err := s.queries.ListSessionSpellCasts(ctx, playdb.ListSessionSpellCastsParams{GameSessionID: session.ID, Limit: maxCastsShown})
	if err != nil {
		return nil, s.dbError(ctx, "list the casts", err)
	}
	lens, err := s.lensFor(ctx, m, append(slices.Clone(live), logged...))
	if err != nil {
		return nil, s.dbError(ctx, "list the casts", err)
	}
	return connect.NewResponse(lens.listing(only, live, logged)), nil
}

// openSessionOf is the campaign's open session, or the error that there is none.
func (s *Service) openSessionOf(ctx context.Context, campaignID string) (playdb.GameSession, error) {
	session, err := s.queries.GetOpenGameSession(ctx, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return playdb.GameSession{}, errNoOpenSession()
	}
	if err != nil {
		return playdb.GameSession{}, s.dbError(ctx, "find the open session", err)
	}
	return session, nil
}

// castsAbout is the character the list is limited to: "" for none. A player names an NPC by its place on
// the stage and may ask only about the characters they may see.
func (s *Service) castsAbout(ctx context.Context, m authz.Membership, sessionID, raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	id, ok := uid.Canonical(raw)
	if !ok {
		return "", errCharacterNotFound()
	}
	if m.Role == authz.RoleMaster {
		return id, nil
	}
	return s.playerMaySee(ctx, m, sessionID, id)
}

// listing is what the viewer reads of the live casts and the log, limited to the casts that are about
// the character (it cast them or was a target) when only is set.
func (l castLens) listing(only string, live, logged []playdb.SpellCast) *playv1.ListSpellCastsResponse {
	out := &playv1.ListSpellCastsResponse{}
	about := func(r playdb.SpellCast) bool {
		return l.sees(r) && (only == "" || r.CasterID == only || slices.ContainsFunc(castTargetsOf(r), func(t castTarget) bool { return t.ID == only }))
	}
	for _, r := range live {
		if about(r) {
			out.Active = append(out.Active, l.view(r))
		}
	}
	for _, r := range logged {
		if about(r) {
			out.Log = append(out.Log, l.view(r))
		}
	}
	return out
}

// playerMaySee checks a player may read about the character and returns its character ID:
// the id is a player character's or the place of an NPC on the stage. Any other is not
// found, as one that does not exist (RN-10).
func (s *Service) playerMaySee(ctx context.Context, m authz.Membership, sessionID, id string) (string, error) {
	sm, err := stageMapOf(ctx, s.queries, sessionID, stageViewer(m))
	if err != nil {
		return "", s.dbError(ctx, "list the stage", err)
	}
	if ch, ok := sm.byEntry[id]; ok {
		return ch, nil
	}
	chars, err := s.roster.CombatCharacters(ctx, nil, m.CampaignID, []string{id})
	if err != nil {
		return "", err
	}
	if len(chars) == 0 || chars[0].CombatOnly || !chars[0].Player {
		return "", errCharacterNotFound()
	}
	return id, nil
}

// GetCastOptions implements playv1connect.CastingServiceHandler.
func (s *Service) GetCastOptions(
	ctx context.Context,
	req *connect.Request[playv1.GetCastOptionsRequest],
) (*connect.Response[playv1.GetCastOptionsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	session, err := s.openSessionOf(ctx, m.CampaignID)
	if err != nil {
		return nil, err
	}
	id, ok := uid.Canonical(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	chars, err := s.roster.CombatCharacters(ctx, nil, m.CampaignID, []string{id})
	if err != nil {
		return nil, err
	}
	if len(chars) == 0 || chars[0].CombatOnly {
		return nil, errCharacterNotFound()
	}
	caster := chars[0]
	if err := mayCastFor(caster, m); err != nil {
		return nil, err
	}
	out := &playv1.GetCastOptionsResponse{}
	if err := s.fillCastOptions(ctx, m, session, id, caster, out); err != nil {
		return nil, err
	}
	if err := s.fillLiveCasts(ctx, m, id, out); err != nil {
		return nil, err
	}
	return connect.NewResponse(out), nil
}

// fillCastOptions adds the spells the character can cast, whether it is in a combat or in a beast
// form, and the targets it can pick with how far each spell reaches them.
func (s *Service) fillCastOptions(ctx context.Context, m authz.Membership, session playdb.GameSession, id string, caster link.Character, out *playv1.GetCastOptionsResponse) error {
	var err error
	if out.Spells, err = s.roster.CastingOptions(ctx, nil, m.CampaignID, id); err != nil {
		return err
	}
	if out.InCombat, err = s.inCombat(ctx, s.queries, session.ID, id); err != nil {
		return s.dbError(ctx, "find the combat", err)
	}
	if caster.Player {
		v, err := s.vitals.GetVitals(ctx, m.CampaignID, id)
		if err != nil {
			return err
		}
		out.WildShape = noSpellsIn(v, caster.CastsInBeastForm) != nil
	}
	out.ArmorNotProficient = caster.ArmorUntrained
	if out.Targets, err = s.castTargetsFor(ctx, m, session, id); err != nil {
		return err
	}
	for _, sp := range out.Spells {
		if sp.Reach, err = s.reachOfTargets(ctx, m.CampaignID, sp, id, out.Targets); err != nil {
			return err
		}
	}
	return nil
}

// fillLiveCasts adds the casting the character is making and the concentration it holds.
func (s *Service) fillLiveCasts(ctx context.Context, m authz.Membership, id string, out *playv1.GetCastOptionsResponse) error {
	live, err := s.queries.ListLiveSpellCastsOfCaster(ctx, id)
	if err != nil {
		return s.dbError(ctx, "list the caster's casts", err)
	}
	lens, err := s.lensFor(ctx, m, live)
	if err != nil {
		return s.dbError(ctx, "list the caster's casts", err)
	}
	for _, r := range live {
		switch {
		case r.Status == castCasting:
			out.Casting = lens.view(r)
		case r.Concentrating:
			out.Concentrating = lens.view(r)
		}
	}
	return nil
}

// castTargetsFor lists the characters a caller may pick as targets: the player characters of
// the party and the NPCs on the stage, an NPC named by its place on the stage for a player
// (RN-20), with the distance from the caster on the map when both stand on it.
func (s *Service) castTargetsFor(ctx context.Context, m authz.Membership, session playdb.GameSession, casterID string) ([]*playv1.CastingTarget, error) {
	party, err := s.roster.CombatParty(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, err
	}
	pos, err := s.tokenSquares(ctx, nil, session)
	if err != nil {
		return nil, s.dbError(ctx, "read the tokens", err)
	}
	add := func(out []*playv1.CastingTarget, publicID string, ch link.Character) []*playv1.CastingTarget {
		t := &playv1.CastingTarget{CharacterId: publicID, Name: ch.Name, Npc: !ch.Player}
		t.DistanceFt, t.DistanceKnown = between(pos, casterID, ch.ID)
		return append(out, t)
	}
	var out []*playv1.CastingTarget
	for _, p := range party {
		out = add(out, p.ID, p)
	}
	sm, err := stageMapOf(ctx, s.queries, session.ID, stageViewer(m))
	if err != nil {
		return nil, s.dbError(ctx, "list the stage", err)
	}
	if len(sm.byChar) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(sm.byChar))
	for id := range sm.byChar {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	npcs, err := s.roster.CombatCharacters(ctx, nil, m.CampaignID, ids)
	if err != nil {
		return nil, err
	}
	for _, n := range npcs {
		if n.Player || n.CombatOnly {
			continue
		}
		public := sm.byChar[n.ID]
		if m.Role == authz.RoleMaster {
			public = n.ID
		}
		out = add(out, public, n)
	}
	return out, nil
}
