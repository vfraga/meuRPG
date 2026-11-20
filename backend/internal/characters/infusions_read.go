package characters

import (
	"context"
	"slices"
	"strings"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The reads of InfusionService (infusions.proto): what a character keeps live and knows, and the pieces an infusion can go
// on. The writes are in infusions_write.go. In this version the holder of an infusion is its infuser, so one character's
// records are both the ones it made and the ones its sheet carries.

var _ charactersv1connect.InfusionServiceHandler = (*Service)(nil)

// infusionView is a character as the infusion calls read it: its sheet, the live records that name it as holder and the
// sheet derived with them. raw is the build without the records, which says what the sheet itself holds (a manual magic
// bonus, a weapon marked magical).
type infusionView struct {
	row     charactersdb.Character
	full    *charactersv1.FullSheet
	content *rules.Content
	raw     rules.Build
	build   rules.Build
	records []rules.InfusionRecord
	derived rules.Derived
}

// infusionViewOf reads the records of the character row and derives its sheet with them. A write calls it after it locked
// the row, so the records are the ones it changes against.
func (s *Service) infusionViewOf(ctx context.Context, q *charactersdb.Queries, content *rules.Content, row charactersdb.Character) (infusionView, error) {
	stored, err := loadSheet(row.ID, row.Sheet)
	if err != nil {
		return infusionView{}, err
	}
	full := stored.GetFull()
	if full == nil {
		return infusionView{}, invalidArgument(fieldErr("character_id", "is a character with a basic sheet, which has no gear to infuse"))
	}
	raw := buildWithStates(full, stateKeysOf(row.ActiveStates))
	build, err := s.withInfusions(ctx, q, row.ID, raw)
	if err != nil {
		return infusionView{}, err
	}
	return infusionView{row: row, full: full, content: content, raw: raw, build: build, records: build.Infusions, derived: rules.Derive(build, content)}, nil
}

// readInfusionCharacter finds the character the caller may see (its player and the master), or answers not_found.
func (s *Service) readInfusionCharacter(ctx context.Context, m authz.Membership, rawID string) (charactersdb.Character, error) {
	id, ok := uid.Canonical(rawID)
	if !ok {
		return charactersdb.Character{}, errCharacterNotFound()
	}
	row, err := s.queries.GetCharacter(ctx, charactersdb.GetCharacterParams{CampaignID: m.CampaignID, ID: id})
	if err != nil {
		return row, s.dbError(ctx, "get a character", err)
	}
	if !canSee(m, row.Kind, row.Status, row.PlayerUserID) {
		return row, errCharacterNotFound()
	}
	return row, nil
}

// checkInfusionHolder refuses a holder that is not the infuser: in this version an infusion is only on the infuser's gear.
func checkInfusionHolder(holder, infuser string) error {
	if id, ok := uid.Canonical(holder); !ok || id != infuser {
		return invalidArgument(fieldErr("holder_character_id", "must be the character that infuses"))
	}
	return nil
}

// knownInfusionKeys are the infusions the sheet picked that it knows now, in the order of its choices: the content has
// them, the character has their feature at its level, still meets their prerequisite and picked them. The engine says it by
// reading a record of each on an object: a record counts exactly when the infusion is known (one the limit pushes out is
// still known).
func knownInfusionKeys(content *rules.Content, raw rules.Build, characterID string) []string {
	var picks []string
	for _, k := range raw.FeatureChoices {
		if _, ok := content.Infusion(k); ok && !slices.Contains(picks, k) {
			picks = append(picks, k)
		}
	}
	probe := raw
	probe.CharacterID, probe.Infusions = characterID, nil
	for _, k := range picks {
		probe.Infusions = append(probe.Infusions, rules.InfusionRecord{
			ID: "probe:" + k, ArtificerID: characterID, InfusionKey: k, ItemKind: rules.InfusionItemObject, ItemKey: "object:probe:" + k,
		})
	}
	var out []string
	for _, st := range rules.Derive(probe, content).Infusions {
		if st.Valid || st.OverLimit {
			out = append(out, strings.TrimPrefix(st.ID, "probe:"))
		}
	}
	return out
}

// ListCharacterInfusions implements charactersv1connect.InfusionServiceHandler.
func (s *Service) ListCharacterInfusions(
	ctx context.Context,
	req *connect.Request[charactersv1.ListCharacterInfusionsRequest],
) (*connect.Response[charactersv1.ListCharacterInfusionsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	row, err := s.readInfusionCharacter(ctx, m, req.Msg.GetCharacterId())
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	v, err := s.infusionViewOf(ctx, s.queries, content, row)
	if err != nil {
		return nil, s.dbError(ctx, "read the infusions of a character", err)
	}
	res := &charactersv1.ListCharacterInfusionsResponse{Active: sheetInfusionsOf(row.ID, v.derived, v.records), Known: v.known()}
	// A character with two features that hold infusions has their limits added, and so have the records they hold.
	for _, h := range v.derived.InfusionHosts {
		res.Limit += i32(h.Limit)
		res.Used += i32(h.Used)
	}
	return connect.NewResponse(res), nil
}

// known lists the infusions the character knows and keeps in no live valid record.
func (v infusionView) known() []*charactersv1.KnownInfusion {
	inUse := map[string]bool{}
	for _, st := range v.derived.Infusions {
		if st.Valid {
			inUse[st.InfusionKey] = true
		}
	}
	var out []*charactersv1.KnownInfusion
	for _, key := range knownInfusionKeys(v.content, v.raw, v.row.ID) {
		if inUse[key] {
			continue
		}
		out = append(out, v.knownInfusion(key))
	}
	return out
}

// knownInfusion is the option of an infusion as the player reads it.
func (v infusionView) knownInfusion(key string) *charactersv1.KnownInfusion {
	in, _ := v.content.Infusion(key)
	k := &charactersv1.KnownInfusion{
		InfusionKey: key, NamePt: v.content.NamePT(key), DescriptionPt: strings.Join(v.content.FeatureTextPT(key), "\n\n"),
		Accepts: infusionAcceptsToProto[in.Accepts], RequiresAttunement: in.RequiresAttunement, Computed: in.Effect != rules.InfusionEffectNone,
	}
	for _, d := range in.DamageTypes {
		k.Parameters = append(k.Parameters, &charactersv1.InfusionParameterOption{Key: d, NamePt: v.content.NamePT(d)})
	}
	return k
}

// ListInfusionTargets implements charactersv1connect.InfusionServiceHandler.
func (s *Service) ListInfusionTargets(
	ctx context.Context,
	req *connect.Request[charactersv1.ListInfusionTargetsRequest],
) (*connect.Response[charactersv1.ListInfusionTargetsResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	row, err := s.readInfusionCharacter(ctx, m, req.Msg.GetCharacterId())
	if err != nil {
		return nil, err
	}
	if err := checkInfusionHolder(req.Msg.GetHolderCharacterId(), row.ID); err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	v, err := s.infusionViewOf(ctx, s.queries, content, row)
	if err != nil {
		return nil, s.dbError(ctx, "read the pieces of a character", err)
	}
	// Only an infusion the character knows has pieces to go on: for any other key the answer is the one of a key that is
	// no infusion at all.
	in, ok := v.knownData(req.Msg.GetInfusionKey())
	if !ok {
		return connect.NewResponse(&charactersv1.ListInfusionTargetsResponse{}), nil
	}
	return connect.NewResponse(&charactersv1.ListInfusionTargetsResponse{Targets: v.targets(in)}), nil
}

// knownData is the data of the infusion when the character knows it now.
func (v infusionView) knownData(key string) (rules.TableInfusion, bool) {
	if !slices.Contains(knownInfusionKeys(v.content, v.raw, v.row.ID), key) {
		return rules.TableInfusion{}, false
	}
	return v.content.Infusion(key)
}

// targets lists the pieces of the sheet an infusion can go on, one row for each gear key (two equal pieces are one row
// with two copies), and the row of an object the player names when the infusion goes on objects.
func (v infusionView) targets(in rules.TableInfusion) []*charactersv1.InfusionTarget {
	taken := map[string]string{}
	for _, st := range v.derived.Infusions {
		if st.Valid && st.ItemKind == rules.InfusionItemGear {
			taken[st.ItemKey] = st.NamePT
		}
	}
	var out []*charactersv1.InfusionTarget
	for _, p := range rules.DeriveGear(v.build, v.content, v.derived) {
		t := &charactersv1.InfusionTarget{ItemKey: p.Key, NamePt: p.NamePT, KindPt: kindWordPT[p.Kind], Copies: i32(p.Count)}
		bonus, magical := rules.MagicalGear(v.raw, p.Key)
		switch {
		case !v.content.InfusionAcceptsGear(in.Accepts, p.Kind, p.Key):
			t.Reason, t.ReasonPt = charactersv1.InfusionTargetReason_INFUSION_TARGET_REASON_WRONG_KIND, wrongKindPT[in.Accepts]
		case magical:
			t.Reason, t.ReasonPt = charactersv1.InfusionTargetReason_INFUSION_TARGET_REASON_ALREADY_MAGICAL, alreadyMagicalPT(bonus)
		case taken[p.Key] != "":
			t.Reason, t.ReasonPt = charactersv1.InfusionTargetReason_INFUSION_TARGET_REASON_TAKEN, takenPT(taken[p.Key])
		}
		out = append(out, t)
	}
	if rules.InfusionTakesObject(in.Accepts) {
		out = append(out, &charactersv1.InfusionTarget{KindPt: objectKindPT})
	}
	return out
}
