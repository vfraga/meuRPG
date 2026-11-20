package characters

import (
	"context"
	"slices"
	"strings"

	"google.golang.org/protobuf/types/known/timestamppb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The loader of the infusion records into a Build. A record names the character whose gear it sits on as its holder;
// the live ones of a holder are Build.Infusions, which the derivation reads. Every read or check that derives numbers
// the infusions change (the sheet a client reads, the save check, the attunement count, the level-up preview, the
// combat's attacks and armor class) builds with them; one that reads none of those numbers builds without (the sites are
// listed in infusions_sites_test.go).

// infusionRecordsOf turns the live rows of a holder into the records the derivation reads, in the order of the rows
// (since, then id).
func infusionRecordsOf(rows []charactersdb.CharacterInfusion) []rules.InfusionRecord {
	out := make([]rules.InfusionRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, rules.InfusionRecord{
			ID: r.ID, InfusionKey: r.InfusionKey, ItemKind: r.ItemKind, ItemKey: r.ItemKey,
			ObjectName: r.ObjectName, ArtificerID: r.ArtificerCharacterID, Parameter: r.Parameter, Attuned: r.Attuned, Since: r.Since,
		})
	}
	return out
}

// holderInfusions reads the live records that name the character as holder. A write calls it after it locked the
// character's row, so the records it reads are the ones the write changes against.
func (s *Service) holderInfusions(ctx context.Context, q *charactersdb.Queries, characterID string) ([]rules.InfusionRecord, error) {
	rows, err := q.ListLiveInfusionsOfHolder(ctx, characterID)
	if err != nil {
		return nil, wrap("read the infusions of a holder", err)
	}
	return infusionRecordsOf(rows), nil
}

// withInfusions is the build with the character's live infusion records in it.
func (s *Service) withInfusions(ctx context.Context, q *charactersdb.Queries, characterID string, b rules.Build) (rules.Build, error) {
	records, err := s.holderInfusions(ctx, q, characterID)
	if err != nil {
		return b, err
	}
	b.CharacterID, b.Infusions = characterID, records
	return b, nil
}

// infusedBuild is buildWithStates and the records of the character: the build every read that derives numbers uses.
func (s *Service) infusedBuild(ctx context.Context, q *charactersdb.Queries, characterID string, f *charactersv1.FullSheet, states []string) (rules.Build, error) {
	return s.withInfusions(ctx, q, characterID, buildWithStates(f, states))
}

var infusionItemKindToProto = map[string]charactersv1.InfusionItemKind{
	rules.InfusionItemGear:   charactersv1.InfusionItemKind_INFUSION_ITEM_KIND_GEAR,
	rules.InfusionItemObject: charactersv1.InfusionItemKind_INFUSION_ITEM_KIND_OBJECT,
}

// sheetInfusionsOf is the infusion list of a sheet's read: one entry per live record of the holder, valid or not, the
// oldest first (since, then id). d is the sheet derived with the same records, which says what each one gives now.
func sheetInfusionsOf(holderID string, d rules.Derived, records []rules.InfusionRecord) []*charactersv1.SheetInfusion {
	status := make(map[string]rules.InfusionStatus, len(d.Infusions))
	for _, st := range d.Infusions {
		status[st.ID] = st
	}
	ordered := slices.Clone(records)
	slices.SortStableFunc(ordered, func(a, b rules.InfusionRecord) int {
		if c := a.Since.Compare(b.Since); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	out := make([]*charactersv1.SheetInfusion, 0, len(ordered))
	for _, r := range ordered {
		st := status[r.ID]
		out = append(out, &charactersv1.SheetInfusion{
			Id: r.ID, InfusionKey: r.InfusionKey, NamePt: st.NamePT, HolderCharacterId: holderID,
			ItemKind: infusionItemKindToProto[r.ItemKind], ItemKey: itemKeyOfGear(r), ObjectNamePt: r.ObjectName, ItemNamePt: st.ItemNamePT,
			Parameter: r.Parameter, Since: timestamppb.New(r.Since), Attuned: r.Attuned,
			RequiresAttunement: st.RequiresAttunement, Computed: st.Computed, ValidNow: st.Valid, Bonus: i32(st.Bonus),
		})
	}
	return out
}

// itemKeyOfGear is the gear key of a gear record, "" for an object (whose key is the record's own).
func itemKeyOfGear(r rules.InfusionRecord) string {
	if r.ItemKind != rules.InfusionItemGear {
		return ""
	}
	return r.ItemKey
}
