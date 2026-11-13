package play

import (
	"context"
	"fmt"
	"strings"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"

	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules/combat"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// The scopes of the sunlight a combat keeps (encounters.sunlight_scope).
const (
	sunScopeMap     = "map"
	sunScopeSquares = "squares"
	// maxSunSquares is what the table's check allows (encounters_sunlight_squares_valid).
	maxSunSquares = 4096
)

// sunlight is the sunlight of a combat as the rolls read it (RN-20, RN-29): who stands in it. The zero value is no
// sunlight, so a caller with no combat at hand lights nobody.
type sunlight struct {
	on bool
	// squaresOnly: only the squares are sunlit; otherwise the whole map (or the whole combat) is.
	squaresOnly bool
	squares     map[grid.Square]bool
	// dark are the squares a magical Darkness covers: the sun does not reach them.
	dark map[grid.Square]bool
}

// lit says whether the combatant stands in sunlight. Darkness beats the sun; an unplaced combatant is in the sun
// of the whole map and in none of the squares.
func (u sunlight) lit(c playdb.Combatant) bool {
	if !u.on {
		return false
	}
	if !placed(c) {
		return !u.squaresOnly
	}
	sq := squareOfCombatant(c)
	if u.dark[sq] {
		return false
	}
	return !u.squaresOnly || u.squares[sq]
}

// facts sets the creature's Sunlit by where the combatant stands.
func (u sunlight) facts(cr combat.Creature, c playdb.Combatant) combat.Creature {
	cr.Sunlit = u.lit(c)
	return cr
}

// sunlightOf reads the sunlight of the combat: nothing is read while it is off. A combat with no grid has no squares,
// so the whole of it is in the sun.
func (s *Service) sunlightOf(ctx context.Context, q *playdb.Queries, enc playdb.Encounter) (sunlight, error) {
	if !enc.SunlightOn {
		return sunlight{}, nil
	}
	var zs []*zone
	if !isTheatre(enc) {
		var err error
		if zs, err = readZonesOf(ctx, q, enc.ID); err != nil {
			return sunlight{}, err
		}
	}
	return sunlightFrom(enc, zs), nil
}

// sunlightFrom is the sunlight of a combat from its row and the zones it keeps: the pure part of sunlightOf, which
// the master's view of the combat reads from the data it already has.
func sunlightFrom(enc playdb.Encounter, zones []*zone) sunlight {
	if !enc.SunlightOn {
		return sunlight{}
	}
	out := sunlight{on: true}
	if isTheatre(enc) {
		return out
	}
	if enc.SunlightScope == sunScopeSquares {
		out.squaresOnly, out.squares = true, squareSet(squaresOfJSON(enc.SunlightSquares))
	}
	for _, z := range zones {
		if z.def.sight != sightDark {
			continue
		}
		if out.dark == nil {
			out.dark = map[grid.Square]bool{}
		}
		for _, sq := range z.squares {
			out.dark[sq] = true
		}
	}
	return out
}

// sunTraitOf is the sunlight trait the stat block of a monster has, none for a key that is not one.
func sunTraitOf(content *rules.Content, monsterKey string) playv1.SunlightTrait {
	d, ok := content.MonsterDerived(monsterKey)
	if !ok {
		return playv1.SunlightTrait_SUNLIGHT_TRAIT_UNSPECIFIED
	}
	for _, f := range d.Features {
		switch {
		case strings.HasSuffix(f.Key, "#sunlight-weakness"):
			return playv1.SunlightTrait_SUNLIGHT_TRAIT_WEAKNESS
		case strings.HasSuffix(f.Key, "#sunlight-sensitivity"):
			return playv1.SunlightTrait_SUNLIGHT_TRAIT_SENSITIVITY
		}
	}
	return playv1.SunlightTrait_SUNLIGHT_TRAIT_UNSPECIFIED
}

// sunlightRead reads the sunlight of the combat the encounter id names (for a caller with no encounter row).
func (s *Service) sunlightRead(ctx context.Context, q *playdb.Queries, encounterID string) (sunlight, error) {
	enc, err := q.GetEncounterByID(ctx, encounterID)
	if err != nil {
		return sunlight{}, fmt.Errorf("read the encounter: %w", err)
	}
	return s.sunlightOf(ctx, q, enc)
}

func squareSet(sq []grid.Square) map[grid.Square]bool {
	out := make(map[grid.Square]bool, len(sq))
	for _, s := range sq {
		out[s] = true
	}
	return out
}
