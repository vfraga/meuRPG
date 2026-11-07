package characters

import (
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

var gearKindToProto = map[string]rulesv1.GearKind{
	rules.GearArmor:  rulesv1.GearKind_GEAR_KIND_ARMOR,
	rules.GearShield: rulesv1.GearKind_GEAR_KIND_SHIELD,
	rules.GearWeapon: rulesv1.GearKind_GEAR_KIND_WEAPON,
}

func gearToProto(g rules.GearPiece) *rulesv1.GearEntry {
	return &rulesv1.GearEntry{
		Key: g.Key, NamePt: g.NamePT, Kind: gearKindToProto[g.Kind], Equipped: g.Equipped,
		TwoHanded: g.TwoHanded, ArmorClassIfToggled: i32(g.ArmorClassIfToggled), Count: i32(g.Count),
	}
}
