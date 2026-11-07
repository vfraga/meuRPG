package characters

import (
	"context"

	"connectrpc.com/connect"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The class editor's two reads (MR-025): the defaults of the 20-level
// table and the closed menu of effects. Both are rules content, so they come from
// the rules package as it is, and the browser computes and hard-codes nothing of
// them. Only the master reads them: they are for the editor.

// GetClassTableDefaults implements rulesv1connect.TableContentServiceHandler.
func (s *Service) GetClassTableDefaults(
	ctx context.Context,
	req *connect.Request[rulesv1.GetClassTableDefaultsRequest],
) (*connect.Response[rulesv1.GetClassTableDefaultsResponse], error) {
	if _, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster); err != nil {
		return nil, err
	}
	// The numbers are the SRD's, the same for every campaign: they come from the SRD
	// itself, so a campaign whose content cannot be read never breaks them.
	d := s.srd.TableDefaults()
	res := &rulesv1.GetClassTableDefaultsResponse{SubclassLevel: i32(d.SubclassLevel)}
	for _, n := range d.ProfBonus {
		res.ProfBonus = append(res.ProfBonus, i32(n))
	}
	for _, n := range d.ASILevels {
		res.AsiLevels = append(res.AsiLevels, i32(n))
	}
	for _, t := range d.Tables {
		out := &rulesv1.CastingTableDefault{
			Kind: t.Kind, Preparation: t.Preparation, StartLevel: i32(t.StartLevel), ReferenceClassKey: t.Reference,
		}
		for _, r := range t.Rows {
			slots := make([]int32, len(r.Slots))
			for i, n := range r.Slots {
				slots[i] = i32(n)
			}
			if r.Slots == [9]int{} {
				slots = nil // the rows before the casting starts have none
			}
			out.Rows = append(out.Rows, &rulesv1.TableClassLevel{
				ProfBonus: i32(r.ProfBonus), CantripsKnown: i32(r.CantripsKnown), SpellsKnown: i32(r.SpellsKnown), Slots: slots,
			})
		}
		res.Tables = append(res.Tables, out)
	}
	return connect.NewResponse(res), nil
}

// GetEffectMenu implements rulesv1connect.TableContentServiceHandler.
func (s *Service) GetEffectMenu(
	ctx context.Context,
	req *connect.Request[rulesv1.GetEffectMenuRequest],
) (*connect.Response[rulesv1.GetEffectMenuResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	// The campaign's own content, for the classes `classLevel` takes.
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	menu := content.EffectMenu()
	res := &rulesv1.GetEffectMenuResponse{
		ClassIndexes:        menu.ClassIndexes,
		MaxFeaturesPerClass: i32(menu.MaxFeaturesPerClass), MaxEffectsPerFeature: i32(menu.MaxEffectsPerFeature),
		MaxTagsPerEffect: i32(menu.MaxTagsPerEffect), ExtraAttackMin: i32(menu.ExtraAttackMin), ExtraAttackMax: i32(menu.ExtraAttackMax),
	}
	for _, t := range menu.Types {
		out := &rulesv1.EffectMenuType{Type: t.Type, NamePt: t.NamePT, HintPt: t.HintPT, FeatOnly: t.FeatOnly}
		for _, f := range t.Fields {
			out.Fields = append(out.Fields, &rulesv1.EffectMenuField{
				Name: f.Name, Required: f.Required, Kind: f.Kind, List: f.List, Min: i32(f.Min), Max: i32(f.Max), AlwaysShown: f.AlwaysShown,
			})
		}
		res.Types = append(res.Types, out)
	}
	for _, l := range menu.Lists {
		res.Lists = append(res.Lists, &rulesv1.EffectMenuList{Name: l.Name, Values: menuValues(l.Values)})
	}
	for _, o := range menu.OptionSets {
		res.OptionSets = append(res.OptionSets, &rulesv1.EffectOptionSet{Key: o.Key, NamePt: o.NamePT, Choose: i32(o.Choose), Options: menuValues(o.Options)})
	}
	for _, h := range menu.Helpers {
		res.Helpers = append(res.Helpers, &rulesv1.FormulaHelper{Call: h.Call, Returns: h.Returns, HintPt: h.HintPT})
	}
	return connect.NewResponse(res), nil
}

func menuValues(vs []rules.MenuValue) []*rulesv1.EffectMenuValue {
	out := make([]*rulesv1.EffectMenuValue, len(vs))
	for i, v := range vs {
		out[i] = &rulesv1.EffectMenuValue{
			Key: v.Key, NamePt: v.NamePT, HintPt: v.HintPT, OnlyModes: v.OnlyModes, Min: i32(v.Min), Max: i32(v.Max), Group: v.Group,
		}
	}
	return out
}
