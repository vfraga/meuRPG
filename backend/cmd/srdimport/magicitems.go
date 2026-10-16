package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/PuraFome/meuRPG/backend/internal/rules/srd51"
)

// magicItemsTotal is the number of entries of 5e-SRD-Magic-Items.json at the
// pinned commit: 239 items and families, and 123 variants.
const magicItemsTotal = 362

// magicItemsSource is an entry of 5e-SRD-Magic-Items.json, the fields we keep.
type magicItemsSource struct {
	Index             string                `json:"index"`
	Name              string                `json:"name"`
	EquipmentCategory ref                   `json:"equipment_category"`
	Rarity            struct{ Name string } `json:"rarity"`
	Variants          []ref                 `json:"variants"`
	Variant           bool                  `json:"variant"`
	Desc              []string              `json:"desc"`
}

var magicCategories = map[string]string{
	"armor": "armor", "weapon": "weapon", "ammunition": "ammunition", "potion": "potion",
	"scroll": "scroll", "ring": "ring", "rod": "rod", "staff": "staff", "wand": "wand",
	"wondrous-items": "wondrous-item",
}

var magicRarities = map[string]string{
	"Common": "common", "Uncommon": "uncommon", "Rare": "rare", "Very Rare": "very_rare",
	"Legendary": "legendary", "Artifact": "artifact", "Varies": "varies",
}

var attunementRE = regexp.MustCompile(`(?i)\(requires attunement(?: ([^)]*))?\)`)

// familyAlsoItem are the families that are an item of their own as well. The
// SRD 5.1 gives the Crystal Ball as "very rare or legendary" and its text calls
// the plain one "a very rare item", while the three variants (mind reading,
// telepathy and true seeing) are legendary. The family is therefore rolled at
// very rare as the plain crystal ball, and its variants at legendary.
var familyAlsoItem = map[string]bool{"crystal-ball": true}

func convertMagicItems(in *inputs) (output, error) {
	rows, err := decode[magicItemsSource](in, "5e-SRD-Magic-Items.json")
	if err != nil {
		return output{}, err
	}
	if len(rows) != magicItemsTotal {
		return output{}, fmt.Errorf("magic items: %d entries, want %d at the pinned commit", len(rows), magicItemsTotal)
	}
	out, err := convertMagicItemRows(rows)
	if err != nil {
		return output{}, err
	}
	return output{"magic-items.json", out}, nil
}

// convertMagicItemRows converts the entries, sorted by key. It checks that
// every variant belongs to exactly one family and every family lists variants
// that exist.
func convertMagicItemRows(rows []magicItemsSource) ([]srd51.MagicItem, error) {
	out := make([]srd51.MagicItem, 0, len(rows))
	seen := map[string]bool{}
	for _, r := range rows {
		it, err := convertMagicItemRow(r, seen)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
		seen[r.Index] = true
	}
	isVariant := map[string]bool{}
	for _, r := range rows {
		isVariant["item:"+r.Index] = r.Variant
	}
	if err := linkMagicVariants(out, isVariant); err != nil {
		return nil, err
	}
	for i := range out {
		if isVariant[out[i].Key] && out[i].VariantOf == "" {
			return nil, fmt.Errorf("magic item %s: a variant of no family", out[i].Key)
		}
	}
	sortByKey(out, func(m srd51.MagicItem) string { return m.Key })
	return out, nil
}

// convertMagicItemRow converts one entry and checks what it can alone: its
// category, rarity and text, that it is new, and its place in a family.
func convertMagicItemRow(r magicItemsSource, seen map[string]bool) (srd51.MagicItem, error) {
	cat, ok := magicCategories[r.EquipmentCategory.Index]
	if !ok {
		return srd51.MagicItem{}, fmt.Errorf("magic item %s: unknown category %q", r.Index, r.EquipmentCategory.Index)
	}
	rarity, ok := magicRarities[r.Rarity.Name]
	if !ok {
		return srd51.MagicItem{}, fmt.Errorf("magic item %s: unknown rarity %q", r.Index, r.Rarity.Name)
	}
	if len(r.Desc) == 0 {
		return srd51.MagicItem{}, fmt.Errorf("magic item %s: no description", r.Index)
	}
	if seen[r.Index] {
		return srd51.MagicItem{}, fmt.Errorf("magic item %s: listed twice", r.Index)
	}
	it := srd51.MagicItem{
		Key: "item:" + r.Index, Name: strings.TrimSpace(r.Name), Category: cat, Rarity: rarity,
		Desc: r.Desc,
	}
	if m := attunementRE.FindStringSubmatch(r.Desc[0]); m != nil {
		it.Attunement = true
		it.AttunementBy = strings.TrimSpace(m[1])
	}
	if err := checkMagicFamily(r, rarity); err != nil {
		return srd51.MagicItem{}, err
	}
	it.Standalone = familyAlsoItem[r.Index]
	for _, v := range r.Variants {
		it.Variants = append(it.Variants, "item:"+v.Index)
	}
	return it, nil
}

// checkMagicFamily checks an entry's variants against its rarity: a rarity
// that varies needs variants, a variant has none, and a family that is also an
// item has variants and a rarity of its own.
func checkMagicFamily(r magicItemsSource, rarity string) error {
	if rarity == "varies" && len(r.Variants) == 0 {
		return fmt.Errorf("magic item %s: rarity varies but it has no variants", r.Index)
	}
	if r.Variant && len(r.Variants) > 0 {
		return fmt.Errorf("magic item %s: a variant with variants", r.Index)
	}
	if familyAlsoItem[r.Index] && (len(r.Variants) == 0 || rarity == "varies") {
		return fmt.Errorf("magic item %s: listed as a family that is also an item, but it is not a family with a rarity", r.Index)
	}
	return nil
}

// linkMagicVariants points each variant at its family, refusing a variant
// that does not exist, is not one, is in two families or has no rarity.
func linkMagicVariants(out []srd51.MagicItem, isVariant map[string]bool) error {
	byKey := map[string]*srd51.MagicItem{}
	for i := range out {
		byKey[out[i].Key] = &out[i]
	}
	for i := range out {
		fam := &out[i]
		for _, vk := range fam.Variants {
			v, ok := byKey[vk]
			switch {
			case !ok:
				return fmt.Errorf("magic item %s: variant %s does not exist", fam.Key, vk)
			case !isVariant[vk]:
				return fmt.Errorf("magic item %s: %s is listed as a variant but is not one", fam.Key, vk)
			case v.VariantOf != "":
				return fmt.Errorf("magic item %s: in two families, %s and %s", vk, v.VariantOf, fam.Key)
			case v.Rarity == "varies":
				return fmt.Errorf("magic item %s: a variant must have a rarity", vk)
			}
			v.VariantOf = fam.Key
		}
	}
	return nil
}
