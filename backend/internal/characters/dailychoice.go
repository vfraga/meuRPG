package characters

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// #580 S3: the picks of a daily choice (a skill or tool until the next rest) and the proficiencies that last until a short
// rest. Both live on the stored sheet; only SetDailyChoice and the rests write them.

// keepDailyState carries over what the server keeps of the daily state in a saved sheet: the stored picks and flag, and none
// the client sent. stored is nil for a new character.
func keepDailyState(stored, saved *charactersv1.FullSheet) {
	saved.DailyPicks = nil
	saved.ShortRested = false
	if stored != nil {
		saved.DailyPicks = proto.CloneOf(stored).DailyPicks
		saved.ShortRested = stored.GetShortRested()
	}
}

// restedState is the daily state after a rest. A short rest ends the picks of the choices that now last until a short rest
// (by the effect's duration today, not the one the pick was made under) and marks the short rest as taken; a long rest ends every
// pick and the mark. A pick whose choice is gone ends too. changed says it differs from the sheet's.
func restedState(full *charactersv1.FullSheet, current []rules.DailyChoice, restBound, long bool) (picks []*charactersv1.DailyPick, shortRested, changed bool) {
	for _, p := range full.GetDailyPicks() {
		i := slices.IndexFunc(current, func(c rules.DailyChoice) bool { return c.ID == p.GetChoiceId() })
		if !long && i >= 0 && current[i].UntilRest == rules.DurationLongRest {
			picks = append(picks, p)
		}
	}
	shortRested = full.GetShortRested()
	switch {
	case long:
		shortRested = false
	case restBound:
		shortRested = true
	}
	return picks, shortRested, len(picks) != len(full.GetDailyPicks()) || shortRested != full.GetShortRested()
}

// mayNeedRestState says the snapshot of a sheet has daily state a rest may change: a pick, the short-rest mark, or a proficiency
// a short rest spends.
func mayNeedRestState(f *charactersv1.FullSheet, content *rules.Content, long bool) bool {
	if f == nil {
		return false
	}
	return len(f.GetDailyPicks()) > 0 || f.GetShortRested() || (!long && rules.Derive(buildOf(f), content).RestBound)
}

// endRestPicks writes the daily state a rest leaves, when it changes it. The snapshot the rest read says whether the sheet may
// need it; the sheet is then locked and read again, so a pick made meanwhile is never overwritten.
func (s *Service) endRestPicks(ctx context.Context, q *charactersdb.Queries, r restRow, long bool) error {
	snap, err := loadSheet(r.row.ID, r.row.Sheet)
	if err != nil {
		return err
	}
	if !mayNeedRestState(snap.GetFull(), r.content, long) {
		return nil
	}
	locked, err := q.GetCharacterForUpdate(ctx, charactersdb.GetCharacterForUpdateParams{CampaignID: r.campaign, ID: r.row.ID})
	if err != nil {
		return wrap("lock the character for the rest", err)
	}
	sheet, err := loadSheet(locked.ID, locked.Sheet)
	if err != nil {
		return err
	}
	full := sheet.GetFull()
	if full == nil {
		return nil
	}
	d := rules.Derive(buildOf(full), r.content)
	picks, shortRested, changed := restedState(full, d.DailyChoices, !long && d.RestBound, long)
	if !changed {
		return nil
	}
	items := make([]json.RawMessage, len(picks))
	for i, p := range picks {
		if items[i], err = storeJSON.Marshal(p); err != nil {
			return wrap("encode the picks", err)
		}
	}
	listJSON, err := json.Marshal(items)
	if err != nil {
		return wrap("encode the picks", err)
	}
	flag := []byte("false")
	if shortRested {
		flag = []byte("true")
	}
	if err := q.SetRestState(ctx, charactersdb.SetRestStateParams{ID: r.row.ID, Picks: listJSON, ShortRested: flag, Now: s.now()}); err != nil {
		return wrap("end the sheet's daily picks", err)
	}
	return nil
}

// SetDailyChoice implements charactersv1connect.CharacterServiceHandler.
func (s *Service) SetDailyChoice(
	ctx context.Context,
	req *connect.Request[charactersv1.SetDailyChoiceRequest],
) (*connect.Response[charactersv1.SetDailyChoiceResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	id, ok := uid.Canonical(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	if req.Msg.GetChoiceId() == "" {
		return nil, invalidArgument(fieldErr("choice_id", "is required"))
	}
	var revision int32
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		revision, err = s.pickIn(ctx, tx, m, id, req.Msg.GetChoiceId(), req.Msg.GetKey())
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "pick a daily choice", err)
	}
	logging.Event(ctx, s.logger, "character.daily_choice", slog.String("character_id", id), slog.String("choice_id", req.Msg.GetChoiceId()), slog.Bool("by_master", isMaster(m)))
	return connect.NewResponse(&charactersv1.SetDailyChoiceResponse{Revision: revision}), nil
}

// pickIn is the transaction of SetDailyChoice: the character locked, the choice found among the sheet's, the pick
// checked by deriving the sheet with it, and the sheet written with only daily_picks changed. A locked sheet takes it.
func (s *Service) pickIn(ctx context.Context, tx pgx.Tx, m authz.Membership, id, choiceID, key string) (int32, error) {
	q := s.queries.WithTx(tx)
	current, err := visibleForUpdate(ctx, q, m, id)
	if err != nil {
		return 0, err
	}
	if current.Status == statusDead {
		return 0, errBlocked(charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD, current.ID)
	}
	stored, err := loadSheet(current.ID, current.Sheet)
	if err != nil {
		return 0, err
	}
	full := stored.GetFull()
	if full == nil {
		return 0, invalidArgument(fieldErr("character_id", "is a character with a basic sheet, which has no daily choices"))
	}
	content, err := s.contentFor(ctx, tx, m.CampaignID)
	if err != nil {
		return 0, wrap("read rules content", err)
	}
	next, err := withDailyPick(content, full, choiceID, key)
	if err != nil {
		return 0, err
	}
	if slices.EqualFunc(next.GetDailyPicks(), full.GetDailyPicks(), func(a, b *charactersv1.DailyPick) bool { return proto.Equal(a, b) }) {
		return current.Revision, nil // already as asked: the revision stays, an open editor is not made stale
	}
	doc, err := storeJSON.Marshal(&charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: next}})
	if err != nil {
		return 0, wrap("encode a sheet", err)
	}
	row, err := q.UpdateCharacterSheet(ctx, charactersdb.UpdateCharacterSheetParams{
		CampaignID: m.CampaignID, ID: current.ID, Revision: current.Revision, Name: current.Name, Sheet: doc, Now: s.now(),
	})
	if err != nil {
		return 0, wrap("update sheet", err)
	}
	return row.Revision, nil
}

// withDailyPick is the sheet with the pick of the choice set to key (taken back when key is empty): invalid_argument for a
// choice the sheet does not have, or a key the choice does not offer.
func withDailyPick(content *rules.Content, full *charactersv1.FullSheet, choiceID, key string) (*charactersv1.FullSheet, error) {
	next := proto.CloneOf(full)
	next.DailyPicks = slices.DeleteFunc(next.DailyPicks, func(p *charactersv1.DailyPick) bool { return p.GetChoiceId() == choiceID })
	idx := slices.IndexFunc(rules.Derive(buildOf(full), content).DailyChoices, func(c rules.DailyChoice) bool { return c.ID == choiceID })
	if idx < 0 {
		return nil, invalidArgument(fieldErr("choice_id", "is not a daily choice of the sheet"))
	}
	if key == "" {
		return next, nil
	}
	choice := rules.Derive(buildOf(full), content).DailyChoices[idx]
	next.DailyPicks = append(next.DailyPicks, &charactersv1.DailyPick{ChoiceId: choiceID, Key: key, UntilRest: restBoundToProto(choice.UntilRest), Signature: choice.Signature})
	got := rules.Derive(buildOf(next), content).DailyChoices
	if i := slices.IndexFunc(got, func(c rules.DailyChoice) bool { return c.ID == choiceID }); i < 0 || got[i].Picked != key {
		return nil, invalidArgument(fieldErr("key", "is not one the daily choice offers"))
	}
	return next, nil
}
