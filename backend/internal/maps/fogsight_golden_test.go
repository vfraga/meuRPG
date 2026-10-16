package maps

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/internal/maps/link"
	"github.com/PuraFome/meuRPG/backend/internal/maps/mapsdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
	"github.com/PuraFome/meuRPG/backend/internal/rules/vision"
)

// Characterization of the scene of a fog map and of the work out of what the players see
// (MR-036, RN-10, D6), called as the services call it: newSight inside and outside a
// transaction, the lights it lists, the familiar's eyes, and updateVision with the
// statements that fail one at a time.

var errInjected = errors.New("injected")

// failQuery is a mapsdb.DBTX that fails the statements of one sqlc query, by name. It
// is not used by a transaction's queries (WithTx puts the transaction in its place).
type failQuery struct {
	mapsdb.DBTX
	name string
}

func (f failQuery) hits(sql string) bool { return strings.Contains(sql, "-- name: "+f.name+" ") }

func (f failQuery) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if f.hits(sql) {
		return pgconn.CommandTag{}, errInjected
	}
	return f.DBTX.Exec(ctx, sql, args...)
}

func (f failQuery) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if f.hits(sql) {
		return nil, errInjected
	}
	return f.DBTX.Query(ctx, sql, args...)
}

func (f failQuery) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if f.hits(sql) {
		return errRow{errInjected}
	}
	return f.DBTX.QueryRow(ctx, sql, args...)
}

// failing makes the service's statements named name fail until the test ends or
// failing("") puts them back.
func (c *cave) failing(name string) {
	c.h.t.Helper()
	if name == "" {
		c.h.svc.queries = mapsdb.New(c.h.pool)
		return
	}
	c.h.svc.queries = mapsdb.New(failQuery{DBTX: c.h.pool, name: name})
}

// fogParts is what newSight reads for the cave.
func (c *cave) fogParts(t *testing.T) (fogInput, []mapsdb.MapPoint, []mapsdb.MapToken) {
	t.Helper()
	in, _, ok, err := c.h.svc.fogRow(t.Context(), c.campaign, c.mapID)
	if err != nil || !ok {
		t.Fatalf("fogRow: ok=%v err=%v", ok, err)
	}
	points, err := c.h.svc.queries.ListMapPoints(t.Context(), c.mapID)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := c.h.svc.queries.ListMapTokens(t.Context(), c.mapID)
	if err != nil {
		t.Fatal(err)
	}
	return in, points, tokens
}

// dumpSight says what a scene holds: who stands where, the extra eyes, the flags, and
// for each user of the cave the revision of the view they have from an empty memory.
func (c *cave) dumpSight(sg *sight, names map[string]string) string {
	var b strings.Builder
	label := func(id string) string {
		if n, ok := names[id]; ok {
			return n
		}
		return "?"
	}
	var stands []string
	for id, sq := range sg.stands {
		stands = append(stands, fmt.Sprintf("%s@(%d,%d)", label(id), sq.Col, sq.Row))
	}
	sort.Strings(stands)
	var extra []string
	for id, vs := range sg.extra {
		for _, v := range vs {
			extra = append(extra, fmt.Sprintf("%s:(%d,%d)/%+v", label(id), v.At.Col, v.At.Row, v.Senses))
		}
	}
	sort.Strings(extra)
	fmt.Fprintf(&b, "grid %dx%d group=%v combat=%v openPlan=%v members=%d\n", sg.g.Columns, sg.g.Rows, sg.group, sg.combat, sg.openPlan, len(sg.members))
	fmt.Fprintf(&b, "stands %s\nextra %s\n", strings.Join(stands, " "), strings.Join(extra, " "))
	for _, u := range []struct {
		name string
		user *user
	}{{"ana", c.ana}, {"caio", c.caio}, {"bia", c.bia}, {"dani", c.dani}, {"master", c.master}} {
		pv := sg.newView(u.user.id, grid.NewLayer(sg.g))
		_, onMap := sg.viewOf(u.user.id)
		fmt.Fprintf(&b, "%s onMap=%v revision=%d\n", u.name, onMap, pv.revision())
	}
	return b.String()
}

// TestFogSight_NewSight: the scene of the cave, built outside a transaction, inside one
// and from the cache, says the same; a grid that is not one is refused; a failed read of the
// layers is the error of the call, and is not kept for the next one.
func TestFogSight_NewSight(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	svc := c.h.svc
	ctx := t.Context()
	in, points, tokens := c.fogParts(t)
	names := map[string]string{
		c.pens.GetId(): "pens", c.toren.GetId(): "toren", c.brisa.GetId(): "brisa", c.salvia.GetId(): "salvia",
		c.goblin1.GetId(): "goblin1", c.goblin2.GetId(): "goblin2", c.captain.GetId(): "captain",
	}

	if sg, err := svc.newSight(ctx, nil, fogInput{}, points, tokens); !errors.Is(err, grid.ErrBadGrid) || sg != nil {
		t.Errorf("newSight with no grid = %v, %v; want ErrBadGrid", sg, err)
	}
	svc.lits.forget(c.mapID)
	cold, err := svc.newSight(ctx, nil, in, points, tokens)
	if err != nil {
		t.Fatalf("newSight() error = %v", err)
	}
	warm, err := svc.newSight(ctx, nil, in, points, tokens)
	if err != nil || warm.entry != cold.entry {
		t.Errorf("a second newSight = %v, %v; want the cached scene", warm, err)
	}
	var inTx *sight
	err = db.InTx(ctx, c.h.pool, func(tx pgx.Tx) error {
		var err error
		inTx, err = svc.newSight(ctx, tx, in, points, tokens)
		return err
	})
	if err != nil || inTx.entry == cold.entry {
		t.Fatalf("newSight in a transaction = %v, %v; want a scene of its own, off the cache", inTx, err)
	}
	dump := c.dumpSight(cold, names)
	if got := c.dumpSight(inTx, names); got != dump {
		t.Errorf("the scene in a transaction says\n%s\nwant what the cached one says\n%s", got, dump)
	}
	checkGolden(t, "fog_sight_cave.golden.txt", []byte(dump))

	// A failed read of the layers: the error of the call, and the cache does not keep the entry.
	c.failing("GetMapLayers")
	svc.lits.forget(c.mapID)
	if sg, err := svc.newSight(ctx, nil, in, points, tokens); err == nil || sg != nil || err.Error() != "read the layers: injected" {
		t.Errorf("newSight with the layers failing = %v, %v; want \"read the layers: injected\"", sg, err)
	}
	c.failing("")
	if sg, err := svc.newSight(ctx, nil, in, points, tokens); err != nil || sg == nil {
		t.Errorf("newSight after the failure = %v, %v; want a scene (the failed entry was dropped)", sg, err)
	}
}

// TestFogSight_LightSources: the Luz points that have a radius, and the light of each
// living character's token, at the square the character stands on; a token whose character is
// gone, whose preset is not one or that stands nowhere lights nothing.
func TestFogSight_LightSources(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	svc := c.h.svc
	ctx := t.Context()
	in, points, _ := c.fogParts(t)
	carry := func(id string, key *string) mapsdb.MapToken {
		return mapsdb.MapToken{MapID: c.mapID, CharacterID: id, CarriedLight: key}
	}
	torch, lantern, nope := "light:torch", "light:hooded-lantern", "light:nope"
	where := func(id string) (grid.Square, bool) {
		switch id {
		case c.pens.GetId():
			return grid.Square{Col: 5, Row: 8}, true
		case c.toren.GetId():
			return grid.Square{Col: 6, Row: 7}, true
		case c.brisa.GetId():
			return grid.Square{Col: 4, Row: 7}, true
		}
		return grid.Square{}, false
	}
	carried := []mapsdb.MapToken{
		carry(c.pens.GetId(), &torch), carry(c.toren.GetId(), &lantern), carry(c.brisa.GetId(), &nope),
		carry(c.salvia.GetId(), &torch),  // stands nowhere
		carry(newKey(), &torch),          // a character that is not there
		carry(c.goblin1.GetId(), nil),    // carries nothing
		carry(c.captain.GetId(), &torch), // stands nowhere
	}
	for name, tc := range map[string]struct {
		points []mapsdb.MapPoint
		tokens []mapsdb.MapToken
	}{
		"no tokens":        {points, nil},
		"no points":        {nil, carried},
		"points and lamps": {points, carried},
		"nothing":          {nil, nil},
	} {
		var got []source
		var err error
		for _, viaTx := range []bool{false, true} {
			if !viaTx {
				got, err = svc.lightSources(ctx, nil, in, tc.points, tc.tokens, where)
			} else {
				var inTx []source
				err = db.InTx(ctx, c.h.pool, func(tx pgx.Tx) error {
					var err error
					inTx, err = svc.lightSources(ctx, tx, in, tc.points, tc.tokens, where)
					return err
				})
				if !slices.Equal(inTx, got) {
					t.Errorf("%s: in a transaction %v, outside %v", name, inTx, got)
				}
			}
			if err != nil {
				t.Fatalf("%s: lightSources() error = %v", name, err)
			}
		}
		checkGolden(t, "fog_lights_"+strings.ReplaceAll(name, " ", "_")+".golden.txt", fmt.Appendf(nil, "%+v\n", got))
	}
}

// TestFogSight_FamiliarEyes: who looks through a familiar, and from which square: a member with
// no eyes or no square on the map has none; the familiar stands where its combatant does, else where
// its token is, and counts only within 20 squares of the character, the straight line.
func TestFogSight_FamiliarEyes(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	svc := c.h.svc
	ctx := t.Context()
	in, _, _ := c.fogParts(t)
	owl := "11111111-1111-4111-8111-111111111111"
	res, err := c.master.characters.GiveCreature(ctx, connect.NewRequest(&charactersv1.GiveCreatureRequest{
		CampaignId: c.campaign, CharacterId: c.salvia.GetId(), MonsterKey: "monster:wolf", Name: "Presa",
	}))
	if err != nil {
		t.Fatalf("GiveCreature() error = %v", err)
	}
	token := res.Msg.GetCreature().GetId()
	place := func(col, row int) {
		x, y := at(col, row)
		if _, err := c.master.maps.PlaceMapToken(ctx, connect.NewRequest(&mapsv1.PlaceMapTokenRequest{CampaignId: c.campaign, MapId: c.mapID, CreatureId: token, XBp: x, YBp: y})); err != nil {
			t.Fatalf("PlaceMapToken(creature) error = %v", err)
		}
	}
	dark := vision.Senses{DarkvisionFt: 60}
	member := func(character, creature string) link.PartyMember {
		return link.PartyMember{CharacterID: character, UserID: "u-" + character, Eyes: &link.FamiliarEyes{CreatureID: creature, Senses: dark}}
	}
	stands := map[string]grid.Square{c.salvia.GetId(): {Col: 2, Row: 8}, c.pens.GetId(): {Col: 5, Row: 8}}
	names := map[string]string{c.salvia.GetId(): "salvia", c.pens.GetId(): "pens", c.toren.GetId(): "toren"}
	run := func(members []link.PartyMember, combat link.CombatPositions) string {
		t.Helper()
		var lines []string
		for _, viaTx := range []bool{false, true} {
			var out map[string][]vision.Viewer
			var err error
			if !viaTx {
				out, err = svc.familiarEyes(ctx, nil, in, members, stands, combat)
			} else {
				err = db.InTx(ctx, c.h.pool, func(tx pgx.Tx) error {
					var err error
					out, err = svc.familiarEyes(ctx, tx, in, members, stands, combat)
					return err
				})
			}
			if err != nil {
				t.Fatalf("familiarEyes() error = %v", err)
			}
			var ids []string
			for id := range out {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			var b strings.Builder
			if out == nil {
				b.WriteString("nil")
			}
			for _, id := range ids {
				fmt.Fprintf(&b, "%s=%v;", names[id], out[id])
			}
			lines = append(lines, b.String())
		}
		if lines[0] != lines[1] {
			t.Errorf("familiarEyes outside a transaction %q, inside %q", lines[0], lines[1])
		}
		return lines[0]
	}
	var sb strings.Builder
	record := func(name, got string) { fmt.Fprintf(&sb, "%s: %s\n", name, got) }

	none := link.CombatPositions{}
	record("no members", run(nil, none))
	record("a member with no eyes", run([]link.PartyMember{{CharacterID: c.salvia.GetId()}}, none))
	record("eyes on a creature with no token", run([]link.PartyMember{member(c.salvia.GetId(), owl)}, none))
	place(18, 6)
	record("the creature is far from a character that stands nowhere", run([]link.PartyMember{member(c.toren.GetId(), token)}, none))
	record("16 across and 2 down", run([]link.PartyMember{member(c.salvia.GetId(), token)}, none))
	place(22, 8) // exactly 20 squares from (2, 8)
	record("exactly 20 squares away", run([]link.PartyMember{member(c.salvia.GetId(), token)}, none))
	place(22, 9)
	record("20 across and 1 down", run([]link.PartyMember{member(c.salvia.GetId(), token)}, none))
	place(14, 15) // 12 across and 7 down
	record("12 across and 7 down", run([]link.PartyMember{member(c.salvia.GetId(), token)}, none))
	place(2, 0)
	record("8 up", run([]link.PartyMember{member(c.salvia.GetId(), token)}, none))
	combat := link.CombatPositions{Running: true, Creatures: map[string]grid.Square{token: {Col: 4, Row: 8}}}
	record("the combatant stands elsewhere than the token", run([]link.PartyMember{member(c.salvia.GetId(), token)}, combat))
	combat.Creatures[token] = grid.Square{Col: 30, Row: 8}
	record("the combatant is out of range, the token in", run([]link.PartyMember{member(c.salvia.GetId(), token)}, combat))
	combat.Creatures[owl] = grid.Square{Col: 6, Row: 8}
	record("two members, one creature each", run([]link.PartyMember{member(c.salvia.GetId(), token), member(c.pens.GetId(), owl)}, combat))
	record("a member twice", run([]link.PartyMember{member(c.salvia.GetId(), owl), member(c.salvia.GetId(), owl)}, combat))
	checkGolden(t, "fog_familiar_eyes.golden.txt", []byte(sb.String()))
}

// TestFogSight_UpdateVision: the work out of what the players see skips a map that is gone, has no
// fog or is one the players do not see (and forgets what it kept of it), remembers nothing for it,
// and returns the error of the statement that failed, in the order it reads them.
func TestFogSight_UpdateVision(t *testing.T) {
	t.Parallel()
	c := newCave(t)
	svc := c.h.svc
	ctx := t.Context()
	prime := func(mapID string) {
		svc.seen.swap(mapID, "someone", 7)
	}
	kept := func(mapID string) bool {
		_, had := svc.seen.swap(mapID, "someone", 7)
		return had
	}

	other := newKey()
	prime(other)
	if err := svc.updateVision(ctx, c.campaign, other, nil); err != nil || kept(other) {
		t.Errorf("a map that is not there: %v, kept %v; want nil and forgotten", err, kept(other))
	}
	prime(c.probeMap)
	if err := svc.updateVision(ctx, c.campaign, c.probeMap, nil); err != nil || kept(c.probeMap) {
		t.Errorf("a map with no fog: %v; want nil and forgotten", err)
	}
	hidden := newCaveWith(t, false)
	prime(hidden.mapID)
	if err := hidden.h.svc.updateVision(ctx, hidden.campaign, hidden.mapID, nil); err != nil || hidden.memoryRows() != 0 {
		t.Errorf("a map the players do not see: %v, %d memories; want nil and none", err, hidden.memoryRows())
	}
	if _, had := hidden.h.svc.seen.swap(hidden.mapID, "someone", 7); had {
		t.Error("a map the players do not see was not forgotten")
	}

	for _, name := range []string{"GetMap", "GetMapGrid", "ListMapPoints", "ListMapTokens", "ListMapVisionMemory"} {
		c.failing(name)
		err := svc.updateVision(ctx, c.campaign, c.mapID, nil)
		c.failing("")
		want := map[string]string{
			"GetMap": "find the map: injected", "GetMapGrid": "read the map's grid: injected", "ListMapPoints": "list the points: injected",
			"ListMapTokens": "list the tokens: injected", "ListMapVisionMemory": "read what the players saw: injected",
		}[name]
		if err == nil || err.Error() != want {
			t.Errorf("updateVision with %s failing = %v, want %q", name, err, want)
		}
	}

	// The scene's own failure comes back as it is.
	c.failing("GetMapLayers")
	svc.lits.forget(c.mapID)
	if err := svc.updateVision(ctx, c.campaign, c.mapID, nil); err == nil || err.Error() != "read the layers: injected" {
		t.Errorf("updateVision with the layers failing = %v", err)
	}
	c.failing("")

	// Whoever sees a square of watch is told, and the memory of each player is stored.
	before := c.memoryRows()
	if err := svc.updateVision(ctx, c.campaign, c.mapID, []grid.Square{sqPens}); err != nil {
		t.Fatalf("updateVision() error = %v", err)
	}
	if after := c.memoryRows(); after < before || after == 0 {
		t.Errorf("memories before %d and after %d; want the players' stored", before, after)
	}
}
