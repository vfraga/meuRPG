package leaktest

import (
	"slices"
	"strings"
	"testing"
	"time"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The zones that block sight (Fog Cloud, Darkness, Stinking Cloud) change what a player's map shows (RN-10, RN-37). These
// rows read the map as a player does: GetMap (the tokens), GetMapVision (the fog squares and the remembered ones),
// GetEncounter and the combat log, and the live stream. Each has a canary (the name, the ids and the zone the player may
// never receive) and positive controls (the master, and a player whose character truly sees, who DO receive them), so a
// row cannot pass by reading nothing. Every row fails if the squares of the cloud or the Darkness are ignored by the fog.

// zoneTable is a dim, open fog map with a combat on it: Ana's gnome wizard (darkvision), Caio's warlock with Devil's
// Sight, a goblin in plain sight and, when asked, a hidden wizard NPC.
type zoneTable struct {
	*world
	enc                                      *playv1.Encounter
	wizard, warlock, foe, caster             *charactersv1.Character
	foeName, casterName                      string
	wizardC, warlockC, foeC, casterC, zoneID string
}

const (
	wizardCol, wizardRow   = 3, 5
	warlockCol, warlockRow = 3, 9
	foeCol, foeRow         = 9, 5
	casterCol, casterRow   = 14, 5
)

// hiddenCaster is the hidden NPC wizard of the fixture: its level and the spells it knows.
type hiddenCaster struct {
	level  int32
	spells []string
}

// fogCaster is the wizard of the Fog Cloud rows.
var fogCaster = &hiddenCaster{level: 5, spells: []string{"spell:fog-cloud"}}

// wallCaster is a wizard of the level that casts Wall of Fire (4th-level slot), and Silence.
var wallCaster = &hiddenCaster{level: 7, spells: []string{"spell:wall-of-fire", "spell:silence"}}

// spikeCaster is a caster of the level that casts Spike Growth (2nd-level slot).
var spikeCaster = &hiddenCaster{level: 5, spells: []string{"spell:spike-growth"}}

// newZoneTable builds the fixture. withCaster adds a hidden NPC wizard that knows Fog Cloud.
func newZoneTable(t *testing.T, withCaster bool) *zoneTable {
	t.Helper()
	if withCaster {
		return newZoneTableOf(t, fogCaster)
	}
	return newZoneTableOf(t, nil)
}

// newZoneTableOf is newZoneTable with the hidden caster given (nil for none).
func newZoneTableOf(t *testing.T, caster *hiddenCaster) *zoneTable {
	t.Helper()
	withCaster := caster != nil
	w := &world{stack: newStack(t), secrets: newSecrets(), pts: map[string]*mapsv1.MapPoint{}, puzzles: map[string]*playv1.Puzzle{}, notes: map[string]string{}}
	z := &zoneTable{world: w}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("building the fixture: %v", r)
		}
	}()
	ctx := t.Context()
	w.buildPeople()
	m := w.master
	w.imgMap = w.image("zone-map-image", 240, 160)
	w.fogMap = must(m.maps.CreateMap(ctx, rq(&mapsv1.CreateMapRequest{CampaignId: w.campaign, Name: w.secrets.public("zone-map-name"), ImageId: w.imgMap}))).GetMap().GetId()
	must(m.maps.SetMapRevealed(ctx, rq(&mapsv1.SetMapRevealedRequest{CampaignId: w.campaign, MapId: w.fogMap, Revealed: true})))
	must(m.maps.SetMapGrid(ctx, rq(&mapsv1.SetMapGridRequest{CampaignId: w.campaign, MapId: w.fogMap, Columns: caveColumns})))
	must(m.maps.SetMapFog(ctx, rq(&mapsv1.SetMapFogRequest{CampaignId: w.campaign, MapId: w.fogMap, FogEnabled: new(true), GroupVision: new(false), BaseLight: mapsv1.LightLevel_LIGHT_LEVEL_DIM.Enum()})))
	w.session = must(m.play.StartGameSession(ctx, rq(&playv1.StartGameSessionRequest{CampaignId: w.campaign}))).GetGameSession().GetId()
	must(m.play.SetCurrentMap(ctx, rq(&playv1.SetCurrentMapRequest{CampaignId: w.campaign, MapId: w.fogMap})))

	spells := []string{"spell:fog-cloud", "spell:darkness"}
	z.wizard = must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Pensantus",
		Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8},
			RaceKey:    "race:gnome", Classes: []*charactersv1.ClassLevel{{ClassKey: "class:wizard", Level: 5}},
			KnownSpellKeys: spells, PreparedSpellKeys: spells,
		}}},
	}))).GetCharacter()
	// A warlock with the Devil's Sight invocation: it sees in Darkness to 120 feet.
	z.warlock = must(w.caio.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Vael",
		Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
			BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 10, Wisdom: 10, Charisma: 16},
			RaceKey:    "race:human", Classes: []*charactersv1.ClassLevel{{ClassKey: "class:warlock", Level: 5}},
			FeatureChoiceKeys: []string{"feature:pact-of-the-chain", "feature:eldritch-invocation-armor-of-shadows", "feature:eldritch-invocation-devils-sight"},
			CantripKeys:       []string{"spell:mage-hand", "spell:chill-touch"},
		}}},
	}))).GetCharacter()
	z.foe, z.foeName = w.npc("zone-foe", &charactersv1.BasicSheet{HitPointsMax: 40, ArmorClass: 12, SpeedFt: 30})
	w.allow("zone-foe-name", w.ana, w.caio)
	participants := []*playv1.Participant{{CharacterId: z.foe.GetId(), Hidden: new(false)}}
	if withCaster {
		z.casterName = w.secrets.marker("zone-caster-name")
		z.caster = must(m.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
			CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, Name: z.casterName,
			Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
				BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8},
				RaceKey:    "race:human", Classes: []*charactersv1.ClassLevel{{ClassKey: "class:wizard", Level: caster.level}},
				KnownSpellKeys: caster.spells, PreparedSpellKeys: caster.spells,
			}}},
		}))).GetCharacter()
		participants = append(participants, &playv1.Participant{CharacterId: z.caster.GetId(), Hidden: new(true)})
	}
	e := must(m.combat.StartEncounter(ctx, rq(&playv1.StartEncounterRequest{
		CampaignId: w.campaign, IdempotencyKey: newKey(), Name: w.secrets.public("zone-encounter"), Participants: participants,
	}))).GetEncounter()
	for _, c := range e.GetCombatants() {
		switch c.GetCharacterId() {
		case z.foe.GetId():
			z.foeC = c.GetId()
		case z.wizard.GetId():
			z.wizardC = c.GetId()
		case z.warlock.GetId():
			z.warlockC = c.GetId()
		}
		if z.caster != nil && c.GetCharacterId() == z.caster.GetId() {
			z.casterC = c.GetId()
		}
		if c.GetKind() == playv1.CombatantKind_COMBATANT_KIND_PLAYER {
			must(m.combat.SubmitInitiative(ctx, rq(&playv1.SubmitInitiativeRequest{
				CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: c.GetId(), IdempotencyKey: newKey(),
				Roll: &playv1.SubmitInitiativeRequest_D20Face{D20Face: 12},
			})))
		}
	}
	z.enc = e
	z.put(z.wizardC, wizardCol, wizardRow)
	z.put(z.warlockC, warlockCol, warlockRow)
	z.put(z.foeC, foeCol, foeRow)
	w.place(w.fogMap, z.wizard.GetId(), wizardCol, wizardRow)
	w.place(w.fogMap, z.warlock.GetId(), warlockCol, warlockRow)
	w.place(w.fogMap, z.foe.GetId(), foeCol, foeRow)
	w.showToken(w.fogMap, z.foe.GetId())
	if withCaster {
		z.put(z.casterC, casterCol, casterRow)
		w.place(w.fogMap, z.caster.GetId(), casterCol, casterRow)
	}
	z.enc = must(m.combat.BeginCombat(ctx, rq(&playv1.BeginCombatRequest{CampaignId: w.campaign, EncounterId: e.GetId(), IdempotencyKey: newKey()}))).GetEncounter()
	return z
}

// put places a combatant on the grid before the combat begins.
func (z *zoneTable) put(combatantID string, col, row int32) {
	must(z.master.combat.MoveCombatant(z.t.Context(), rq(&playv1.MoveCombatantRequest{
		CampaignId: z.campaign, EncounterId: z.enc.GetId(), CombatantId: combatantID, IdempotencyKey: newKey(), Col: col, Row: row,
	})))
}

// forceMove has the master move a combatant, the forced way.
func (z *zoneTable) forceMove(combatantID string, col, row int32) {
	must(z.master.combat.MoveCombatant(z.t.Context(), rq(&playv1.MoveCombatantRequest{
		CampaignId: z.campaign, EncounterId: z.enc.GetId(), CombatantId: combatantID, IdempotencyKey: newKey(), Col: col, Row: row, Forced: true,
	})))
}

// untilTurn passes the turns until it is the combatant's.
func (z *zoneTable) untilTurn(combatantID string) {
	ctx := z.t.Context()
	for range 20 {
		cur := must(z.master.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: z.campaign}))).GetEncounter()
		if cur.GetCurrentCombatantId() == combatantID {
			return
		}
		must(z.master.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: z.campaign, EncounterId: z.enc.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	}
	z.t.Fatalf("the turn never came to %s", combatantID)
}

// cast casts a spell on a point as the caster's own side: a player through their client, the hidden NPC through the master's.
// edit changes the request (a wall's placement).
func (z *zoneTable) cast(by *person, casterCombatant, spell string, slot int32, col, row int32, edit ...func(*playv1.CastSpellRequest)) *playv1.CastSpellResponse {
	req := &playv1.CastSpellRequest{
		CampaignId: z.campaign, EncounterId: z.enc.GetId(), CasterId: casterCombatant, SpellKey: spell, Slot: &playv1.SpellSlot{Level: slot}, IdempotencyKey: newKey(),
		Area: &playv1.CastSpellRequest_Origin{Origin: &playv1.SpellOrigin{Col: col, Row: row}},
	}
	for _, f := range edit {
		f(req)
	}
	return must(by.combat.CastSpell(z.t.Context(), rq(req)))
}

// read is everything of the combat and the map a person receives: the map with its tokens, the combat and its log.
func (z *zoneTable) read(p *person) string {
	ctx := z.t.Context()
	var b strings.Builder
	b.WriteString(jsonOf(must(p.maps.GetMap(ctx, rq(&mapsv1.GetMapRequest{CampaignId: z.campaign, MapId: z.fogMap})))))
	b.WriteString(jsonOf(must(p.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: z.campaign})))))
	b.WriteString(jsonOf(must(p.combat.ListCombatLog(ctx, rq(&playv1.ListCombatLogRequest{CampaignId: z.campaign, EncounterId: z.enc.GetId()})))))
	return b.String()
}

// tokensOf lists the characters whose token the person's map holds. While a combat runs the map holds the players'
// tokens alone: the other creatures are the combat's, at the squares of their combatants (see onTheMap).
func (z *zoneTable) tokensOf(p *person) []string {
	var out []string
	for _, tok := range must(p.maps.GetMap(z.t.Context(), rq(&mapsv1.GetMapRequest{CampaignId: z.campaign, MapId: z.fogMap}))).GetTokens() {
		out = append(out, tok.GetCharacterId())
	}
	return out
}

// onTheMap says whether the person's combat puts the combatant on the grid: the creature's place on the map while a combat runs.
func (z *zoneTable) onTheMap(p *person, combatantID string) bool {
	for _, c := range must(p.combat.GetEncounter(z.t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: z.campaign}))).GetEncounter().GetCombatants() {
		if c.GetId() == combatantID && c.GetPlaced() {
			return true
		}
	}
	return false
}

// stateAt is the packed state of a square in the person's view: 0 unseen, 1 a wall, 2 to 4 seen now, 5 remembered.
func stateAt(res *mapsv1.GetMapVisionResponse, col, row int) byte {
	cols := int(res.GetGridColumns())
	i := row*cols + col
	return res.GetStates()[i/2] >> (4 * (i % 2)) & 0xf
}

// seenNow says whether the person's view shows the square now: seen in some light, not a wall and not only remembered.
func seenNow(res *mapsv1.GetMapVisionResponse, col, row int) bool {
	cols := int(res.GetGridColumns())
	i := row*cols + col
	code := res.GetStates()[i/2] >> (4 * (i % 2)) & 0xf
	return code >= 2 && code <= 4
}

// zonesOf is the zones the person's combat holds.
func (z *zoneTable) zonesOf(p *person) []*playv1.SpellZone {
	return must(p.combat.GetEncounter(z.t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: z.campaign}))).GetEncounter().GetZones()
}

// requireClean fails the test for each needle the text holds.
func requireClean(t *testing.T, who, what, text string, needles []string) {
	t.Helper()
	if found := namesAny(text, needles); len(found) > 0 {
		t.Errorf("%s: %s holds %v", who, what, found)
	}
}

// requireHolds fails the test when the text lacks a needle: the positive control.
func requireHolds(t *testing.T, who, what, text string, needles []string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(text, n) {
			t.Fatalf("%s: %s does not hold %q, so the scan proves nothing", who, what, n)
		}
	}
}

// streamsOf opens the stream of the master and both players.
func (z *zoneTable) streamsOf() map[*person]*streamWatcher {
	out := map[*person]*streamWatcher{}
	for _, p := range []*person{z.master, z.ana, z.caio} {
		out[p] = z.watchStream(p)
	}
	return out
}

// endAndCollect ends the combat and the session, waits for the streams to end and returns what each received.
func (z *zoneTable) endAndCollect(watchers map[*person]*streamWatcher) map[*person][]*playv1.WatchGameSessionResponse {
	ctx := z.t.Context()
	must(z.master.combat.EndEncounter(ctx, rq(&playv1.EndEncounterRequest{CampaignId: z.campaign, EncounterId: z.enc.GetId(), IdempotencyKey: newKey()})))
	must(z.master.play.EndGameSession(ctx, rq(&playv1.EndGameSessionRequest{CampaignId: z.campaign, GameSessionId: z.session})))
	out := map[*person][]*playv1.WatchGameSessionResponse{}
	for p, sw := range watchers {
		select {
		case <-sw.done:
		case <-time.After(30 * time.Second):
			z.t.Fatalf("%s's stream did not end with the session", p.name)
		}
		out[p] = sw.since(0)
	}
	return out
}

// moved lists the combatants a stream says moved, and the characters whose token moved.
func moved(events []*playv1.WatchGameSessionResponse) (combatants, tokens []string) {
	for _, ev := range events {
		if m := ev.GetCombatantMoved(); m != nil {
			combatants = append(combatants, m.GetCombatantId())
		}
		if m := ev.GetTokenMoved(); m != nil {
			tokens = append(tokens, m.GetCharacterId())
		}
	}
	return combatants, tokens
}

// bytesOf is the events as text, for the scan.
func bytesOf(events []*playv1.WatchGameSessionResponse) string {
	var b strings.Builder
	for _, ev := range events {
		b.Write(mustJSON(ev))
	}
	return b.String()
}

// A Darkness on a creature takes it off the map of a player who has only darkvision: no token, no square, nothing in the
// combat and no word in the stream. The master's map and the map of the warlock whose Devil's Sight sees in the Darkness
// are the positive controls: they keep the creature, and hear it move.
func TestADarknessTakesACreatureOffTheMapOfADarkvisionPlayer(t *testing.T) {
	z := newZoneTable(t, false)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%v", r)
		}
	}()
	ana, caio, master := z.ana, z.caio, z.master
	needles := []string{z.foeName, z.foe.GetId(), z.foeC}

	// Before the Darkness both players read the creature on the map, and so do the master.
	for _, p := range []*person{ana, caio, master} {
		requireHolds(t, p.name, "the combat before the Darkness", z.read(p), []string{z.foeC})
		if !z.onTheMap(p, z.foeC) {
			t.Fatalf("%s's combat does not place the creature before the Darkness", p.name)
		}
	}
	watchers := z.streamsOf()

	z.untilTurn(z.wizardC)
	cast := z.cast(ana, z.wizardC, "spell:darkness", 2, foeCol, foeRow)
	requireClean(t, ana.name, "the answer to the cast", jsonOf(cast), needles)

	// The darkvision player: the creature is gone from the map, the combat and the log; its square and the Darkness are not seen now.
	read := z.read(ana)
	requireClean(t, ana.name, "the map and the combat in the Darkness", read, needles)
	if z.onTheMap(ana, z.foeC) || slices.Contains(z.tokensOf(ana), z.foe.GetId()) {
		t.Errorf("Ana's map still places the creature in the Darkness")
	}
	view := z.vision(ana, z.fogMap)
	requireClean(t, ana.name, "the answer of GetMapVision", jsonOf(view), needles)
	for _, sq := range [][2]int{{foeCol, foeRow}, {foeCol - 1, foeRow}, {foeCol + 1, foeRow}} {
		if seenNow(view, sq[0], sq[1]) {
			t.Errorf("Ana sees (%d,%d) in the Darkness with darkvision\n%s", sq[0], sq[1], strings.Join(picture(view), "\n"))
		}
	}
	if !seenNow(view, foeCol-5, foeRow) {
		t.Errorf("Ana does not see a square outside the Darkness: the row proves nothing\n%s", strings.Join(picture(view), "\n"))
	}

	// The positive controls: the master, and the warlock whose Devil's Sight sees in the Darkness, still read the creature there.
	requireHolds(t, master.name, "the combat in the Darkness", z.read(master), []string{z.foeC})
	requireHolds(t, caio.name, "the combat of Devil's Sight in the Darkness", z.read(caio), []string{z.foeC})
	if !z.onTheMap(caio, z.foeC) || !z.onTheMap(master, z.foeC) {
		t.Errorf("the Devil's Sight warlock and the master do not read the creature in the Darkness")
	}
	if !seenNow(z.vision(caio, z.fogMap), foeCol, foeRow) {
		t.Errorf("Caio's Devil's Sight does not see the Darkness square")
	}

	// The creature moves inside the Darkness: the stream of the warlock and of the master say where, Ana's says nothing of it.
	z.forceMove(z.foeC, foeCol+1, foeRow)
	events := z.endAndCollect(watchers)
	requireClean(t, ana.name, "the stream", bytesOf(events[ana]), needles)
	if combatants, tokens := moved(events[ana]); slices.Contains(combatants, z.foeC) || slices.Contains(tokens, z.foe.GetId()) {
		t.Errorf("%s's stream says the creature moved: %v %v", ana.name, combatants, tokens)
	}
	for _, p := range []*person{caio, master} {
		if combatants, _ := moved(events[p]); !slices.Contains(combatants, z.foeC) {
			t.Errorf("%s's stream does not say the creature moved in the Darkness, so the scan of Ana's proves nothing: %v", p.name, combatants)
		}
	}
}

// A Fog Cloud cast by a hidden NPC: no player receives the zone, the caster or anything of them, in the combat, on the map
// or in the stream. All they get is the fog's own change: the squares of the cloud, and what is behind it, are not seen now.
// The master's combat holds the zone and its caster: the positive control.
func TestAHiddenCastersFogCloudReachesNoPlayerButTheUnseenSquares(t *testing.T) {
	z := newZoneTable(t, true)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%v", r)
		}
	}()
	ana, caio, master := z.ana, z.caio, z.master
	watchers := z.streamsOf()

	z.untilTurn(z.casterC)
	cast := z.cast(master, z.casterC, "spell:fog-cloud", 1, foeCol, foeRow)
	zones := cast.GetEncounter().GetZones()
	if len(zones) != 1 {
		t.Fatalf("the master's cast holds %d zones, want the cloud", len(zones))
	}
	zone := zones[0]
	z.zoneID = zone.GetId()
	needles := []string{z.casterName, z.caster.GetId(), z.casterC, z.zoneID}

	requireHolds(t, master.name, "the master's combat with the cloud", z.read(master), needles)
	if zs := z.zonesOf(master); len(zs) != 1 || zs[0].GetVisibility() != playv1.SpellZoneVisibility_SPELL_ZONE_VISIBILITY_MASTER_ONLY {
		t.Fatalf("the master reads zones %v, want the hidden caster's cloud marked for the master alone", zs)
	}
	if len(zone.GetSquares()) == 0 {
		t.Fatal("the cloud has no squares")
	}
	for _, p := range []*person{ana, caio} {
		if zs := z.zonesOf(p); len(zs) != 0 {
			t.Errorf("%s reads the zones %v of a hidden caster", p.name, zs)
		}
		requireClean(t, p.name, "the map and the combat with the hidden caster's cloud", z.read(p), needles)
		requireClean(t, p.name, "the answer of GetMapVision", jsonOf(z.vision(p, z.fogMap)), needles)
		view := z.vision(p, z.fogMap)
		for _, sq := range zone.GetSquares() {
			if seenNow(view, int(sq.GetCol()), int(sq.GetRow())) {
				t.Errorf("%s sees (%d,%d) of the cloud now\n%s", p.name, sq.GetCol(), sq.GetRow(), strings.Join(picture(view), "\n"))
			}
		}
		// What the player saw before the cloud is only remembered now (grey), and nothing of it is seen.
		for _, sq := range zone.GetSquares() {
			if got := stateAt(view, int(sq.GetCol()), int(sq.GetRow())); got != 5 {
				t.Errorf("%s: state of the cloud square (%d,%d) = %d, want 5 (seen before the cloud, remembered)", p.name, sq.GetCol(), sq.GetRow(), got)
			}
		}
		// Behind the cloud, on the line from the player's character, nothing is seen either.
		if seenNow(view, casterCol, casterRow) {
			t.Errorf("%s sees (%d,%d), behind the cloud", p.name, casterCol, casterRow)
		}
		if z.onTheMap(p, z.foeC) || slices.Contains(z.tokensOf(p), z.foe.GetId()) || slices.Contains(z.tokensOf(p), z.caster.GetId()) {
			t.Errorf("%s's map places the creature in the cloud or the hidden caster", p.name)
		}
	}
	// A square before the cloud is seen: the cloud is what hides, not the fog of the whole map.
	if !seenNow(z.vision(ana, z.fogMap), foeCol-6, foeRow) {
		t.Error("Ana does not see a square before the cloud: the row proves nothing")
	}

	z.forceMove(z.casterC, casterCol+1, casterRow)
	events := z.endAndCollect(watchers)
	for _, p := range []*person{ana, caio} {
		requireClean(t, p.name, "the stream", bytesOf(events[p]), needles)
		if combatants, tokens := moved(events[p]); slices.Contains(combatants, z.casterC) || slices.Contains(tokens, z.caster.GetId()) {
			t.Errorf("%s's stream says the hidden caster moved: %v %v", p.name, combatants, tokens)
		}
	}
	if combatants, _ := moved(events[master]); !slices.Contains(combatants, z.casterC) {
		t.Errorf("the master's stream does not say the hidden caster moved: %v", combatants)
	}
}

// A square the player never saw is not remembered: the cloud goes up, the master clears what the players remember, and the
// squares of the cloud (never seen since) stay unseen, neither seen nor remembered. When the cloud ends they are seen now.
// Before the clearing they were seen and are remembered (the row above), so the clearing is what makes them never seen.
func TestASquareNeverSeenBecauseOfACloudIsNotRemembered(t *testing.T) {
	z := newZoneTable(t, true)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%v", r)
		}
	}()
	ctx := t.Context()
	z.untilTurn(z.casterC)
	zone := z.cast(z.master, z.casterC, "spell:fog-cloud", 1, foeCol, foeRow).GetEncounter().GetZones()[0]
	must(z.master.maps.ForgetMapVision(ctx, rq(&mapsv1.ForgetMapVisionRequest{CampaignId: z.campaign, MapId: z.fogMap})))
	for _, p := range []*person{z.ana, z.caio} {
		view := z.vision(p, z.fogMap)
		if got := stateAt(view, foeCol-6, foeRow); got < 2 || got > 4 {
			t.Fatalf("%s: a square before the cloud has state %d, want seen now: the row proves nothing", p.name, got)
		}
		for _, sq := range zone.GetSquares() {
			if got := stateAt(view, int(sq.GetCol()), int(sq.GetRow())); got != 0 {
				t.Errorf("%s: the never-seen cloud square (%d,%d) has state %d, want 0 (not seen, not remembered)\n%s", p.name, sq.GetCol(), sq.GetRow(), got, strings.Join(picture(view), "\n"))
			}
		}
	}
	must(z.master.combat.EndSpellZone(ctx, rq(&playv1.EndSpellZoneRequest{CampaignId: z.campaign, EncounterId: z.enc.GetId(), ZoneId: zone.GetId(), IdempotencyKey: newKey()})))
	for _, p := range []*person{z.ana, z.caio} {
		view := z.vision(p, z.fogMap)
		for _, sq := range zone.GetSquares() {
			if !seenNow(view, int(sq.GetCol()), int(sq.GetRow())) {
				t.Errorf("%s: the cloud square (%d,%d) is not seen after the cloud ended", p.name, sq.GetCol(), sq.GetRow())
			}
		}
	}
}

// A cloud that ends: while it stands the squares are hidden and the stream of the players carries only hints; when the
// master ends it the squares are seen again and the stream carries nothing new: no zone id, no caster or creature (name, id, combatant), no move,
// and the only fog hint names the map. The streams are opened with the cloud up, so everything they hold is what a player gets from here on. The sight
// zones are fixed (Fog Cloud, Darkness and Stinking Cloud never move), so what moves is what is inside: the creature and the
// hidden caster move in the cloud with the master's hand.
func TestACloudThatEndsLeavesNothingNewInWhatThePlayersReceive(t *testing.T) {
	z := newZoneTable(t, true)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%v", r)
		}
	}()
	ana, caio, master := z.ana, z.caio, z.master
	z.untilTurn(z.casterC)
	cast := z.cast(master, z.casterC, "spell:fog-cloud", 1, foeCol, foeRow)
	zone := cast.GetEncounter().GetZones()[0]
	z.zoneID = zone.GetId()
	needles := []string{z.casterName, z.caster.GetId(), z.casterC, z.zoneID, z.foeName, z.foe.GetId(), z.foeC}
	watchers := z.streamsOf()

	cloud := func(p *person, want bool) {
		t.Helper()
		view := z.vision(p, z.fogMap)
		for _, sq := range zone.GetSquares() {
			if got := seenNow(view, int(sq.GetCol()), int(sq.GetRow())); got != want {
				t.Fatalf("%s sees (%d,%d) = %v with the cloud up = %v\n%s", p.name, sq.GetCol(), sq.GetRow(), got, !want, strings.Join(picture(view), "\n"))
			}
		}
	}
	for _, p := range []*person{ana, caio} {
		cloud(p, false)
	}

	// Inside the cloud the creature and the hidden caster move; the players hear nothing of where.
	z.forceMove(z.foeC, foeCol+1, foeRow)
	z.forceMove(z.casterC, casterCol-1, casterRow)

	// The master ends the cloud: the players read the squares again, and no zone has ever reached them.
	must(master.combat.EndSpellZone(z.t.Context(), rq(&playv1.EndSpellZoneRequest{CampaignId: z.campaign, EncounterId: z.enc.GetId(), ZoneId: z.zoneID, IdempotencyKey: newKey()})))
	for _, p := range []*person{ana, caio} {
		cloud(p, true)
		if zs := z.zonesOf(p); len(zs) != 0 {
			t.Errorf("%s reads the zones %v after the cloud ended", p.name, zs)
		}
	}
	if zs := z.zonesOf(master); len(zs) != 0 {
		t.Errorf("the master still reads the zones %v after the cloud ended", zs)
	}

	events := z.endAndCollect(watchers)
	for _, p := range []*person{ana, caio} {
		text := bytesOf(events[p])
		requireClean(t, p.name, "the stream after the cloud", text, needles)
		if combatants, tokens := moved(events[p]); len(combatants) > 0 || len(tokens) > 0 {
			t.Errorf("%s's stream says %v %v moved in or behind the cloud", p.name, combatants, tokens)
		}
		hints := 0
		for _, ev := range events[p] {
			if v := ev.GetVisionChanged(); v != nil {
				hints++
				if v.GetMapId() != z.fogMap {
					t.Errorf("%s's vision_changed names the map %q, want the map of the fog %q", p.name, v.GetMapId(), z.fogMap)
				}
			}
		}
		if hints == 0 {
			t.Errorf("%s's stream has no vision_changed after the cloud ended: the row proves nothing", p.name)
		}
	}
	if combatants, _ := moved(events[master]); !slices.Contains(combatants, z.casterC) || !slices.Contains(combatants, z.foeC) {
		t.Errorf("the master's stream does not say who moved in the cloud: %v", combatants)
	}
}

// southWall is the wall from its point to the south, damaging on the west side: the creatures 10 feet west of it are hurt
// at the end of their turn.
func southWall(r *playv1.CastSpellRequest) {
	r.Wall = &playv1.SpellWall{Direction: &playv1.SpellDirection{Dy: 1}, Side: playv1.WallSide_WALL_SIDE_RIGHT}
}

const wallCol, wallRow = 5, 3

// castHiddenWall has the hidden wizard cast Wall of Fire at its turn and returns the zone the master reads.
func (z *zoneTable) castHiddenWall() *playv1.SpellZone {
	z.untilTurn(z.casterC)
	cast := z.cast(z.master, z.casterC, "spell:wall-of-fire", 4, wallCol, wallRow, southWall)
	zones := cast.GetEncounter().GetZones()
	if len(zones) != 1 || len(zones[0].GetSquares()) == 0 {
		z.t.Fatalf("the master's cast holds the zones %v, want the wall with its squares", zones)
	}
	z.zoneID = zones[0].GetId()
	return zones[0]
}

// seenWallSquares are the squares of the zone that the person sees now.
func seenWallSquares(res *mapsv1.GetMapVisionResponse, zone *playv1.SpellZone) []*playv1.SpellOrigin {
	var out []*playv1.SpellOrigin
	for _, sq := range zone.GetSquares() {
		if seenNow(res, int(sq.GetCol()), int(sq.GetRow())) {
			out = append(out, sq)
		}
	}
	return out
}

// A Wall of Fire cast by a hidden NPC: no player receives the zone, the caster or anything of them, in the combat, on the map
// or in the stream, and the wall blocks sight like a Fog Cloud: the squares a player saw before it are not seen now.
// The master's combat holds the zone and its caster, and the player's map the squares before the wall: the positive controls.
func TestAHiddenCastersWallOfFireReachesNoPlayerButTheUnseenSquares(t *testing.T) {
	z := newZoneTableOf(t, wallCaster)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%v", r)
		}
	}()
	ana, caio, master := z.ana, z.caio, z.master
	watchers := z.streamsOf()
	before := map[*person]*mapsv1.GetMapVisionResponse{ana: z.vision(ana, z.fogMap), caio: z.vision(caio, z.fogMap)}

	zone := z.castHiddenWall()
	needles := []string{z.casterName, z.caster.GetId(), z.casterC, z.zoneID}

	requireHolds(t, master.name, "the master's combat with the wall", z.read(master), needles)
	if zs := z.zonesOf(master); len(zs) != 1 || zs[0].GetVisibility() != playv1.SpellZoneVisibility_SPELL_ZONE_VISIBILITY_MASTER_ONLY {
		t.Fatalf("the master reads zones %v, want the hidden caster's wall marked for the master alone", zs)
	}
	for _, p := range []*person{ana, caio} {
		if zs := z.zonesOf(p); len(zs) != 0 {
			t.Errorf("%s reads the zones %v of a hidden caster", p.name, zs)
		}
		requireClean(t, p.name, "the map and the combat with the hidden caster's wall", z.read(p), needles)
		view := z.vision(p, z.fogMap)
		requireClean(t, p.name, "the answer of GetMapVision", jsonOf(view), needles)
		was := seenWallSquares(before[p], zone)
		if len(was) == 0 {
			t.Fatalf("%s saw no square of the wall before the cast, so the scan proves nothing", p.name)
		}
		if now := seenWallSquares(view, zone); len(now) != 0 {
			t.Errorf("%s sees the squares %v of the wall now\n%s", p.name, now, strings.Join(picture(view), "\n"))
		}
		if seenNow(view, foeCol, foeRow) || z.onTheMap(p, z.foeC) {
			t.Errorf("%s still sees the creature behind the wall at (%d,%d)", p.name, foeCol, foeRow)
		}
		if slices.Contains(z.tokensOf(p), z.caster.GetId()) || z.onTheMap(p, z.casterC) {
			t.Errorf("%s's map places the hidden caster", p.name)
		}
		if !seenNow(view, wallCol-2, wallRow+2) {
			t.Errorf("%s does not see a square before the wall: the row proves nothing", p.name)
		}
	}

	z.forceMove(z.casterC, casterCol+1, casterRow)
	events := z.endAndCollect(watchers)
	for _, p := range []*person{ana, caio} {
		requireClean(t, p.name, "the stream", bytesOf(events[p]), needles)
		if combatants, tokens := moved(events[p]); slices.Contains(combatants, z.casterC) || slices.Contains(tokens, z.caster.GetId()) {
			t.Errorf("%s's stream says the hidden caster moved: %v %v", p.name, combatants, tokens)
		}
	}
	if combatants, _ := moved(events[master]); !slices.Contains(combatants, z.casterC) {
		t.Errorf("the master's stream does not say the hidden caster moved: %v", combatants)
	}
}

// The damage a hidden caster's wall leaves to roll at the end of a player's turn names the caster to the master alone:
// the player ends the turn, reads their turn options, and later takes the damage, and nothing they receive holds the caster.
// The master's turn options for the caster, with the damage and its attacker, are the positive control.
func TestTheDamageOfAHiddenCastersWallAtATurnEndNamesNoCasterToThePlayer(t *testing.T) {
	z := newZoneTableOf(t, wallCaster)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%v", r)
		}
	}()
	ctx := t.Context()
	ana, caio, master := z.ana, z.caio, z.master
	watchers := z.streamsOf()
	z.castHiddenWall()
	needles := []string{z.casterName, z.caster.GetId(), z.casterC, z.zoneID}
	opts := func(p *person, combatantID string) *playv1.GetTurnOptionsResponse {
		return must(p.combat.GetTurnOptions(ctx, rq(&playv1.GetTurnOptionsRequest{CampaignId: z.campaign, EncounterId: z.enc.GetId(), CombatantId: combatantID})))
	}

	// Ana's wizard stands 10 feet west of the wall: ending the turn there leaves damage to roll, from the hidden caster.
	z.untilTurn(z.wizardC)
	cur := must(master.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: z.campaign}))).GetEncounter()
	end := must(ana.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: z.campaign, EncounterId: z.enc.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: z.wizardC, ExpectedRound: cur.GetRound()})))
	requireClean(t, ana.name, "the answer to ending the turn", jsonOf(end), needles)

	pending := opts(master, z.casterC).GetPendingDamages()
	if len(pending) != 1 || pending[0].GetAttackerId() != z.casterC || pending[0].GetTargetId() != z.wizardC {
		t.Fatalf("the master's turn options for the caster hold the damages %v, want one on the wizard from the caster", pending)
	}
	reads := func(when string) {
		t.Helper()
		requireClean(t, ana.name, "the map and the combat "+when, z.read(ana), needles)
		requireClean(t, ana.name, "the turn options of her wizard "+when, jsonOf(opts(ana, z.wizardC)), needles)
		requireClean(t, caio.name, "the map and the combat "+when, z.read(caio), needles)
	}
	reads("with the damage to roll")

	// The master rolls it and applies 1 point, so the wizard stays up: the answers the player hears are clean too.
	rolled := must(master.combat.RollDamage(ctx, rq(&playv1.RollDamageRequest{
		CampaignId: z.campaign, EncounterId: z.enc.GetId(), PendingDamageId: pending[0].GetId(), IdempotencyKey: newKey(), Roll: &playv1.RollDamageRequest_RollInApp{RollInApp: true},
	})))
	if rolled.GetPendingDamage().GetAttackerId() != z.casterC {
		t.Errorf("the master's roll does not name the caster as the attacker: %v", rolled.GetPendingDamage())
	}
	one := int32(1)
	must(master.combat.ApplyPendingDamage(ctx, rq(&playv1.ApplyPendingDamageRequest{
		CampaignId: z.campaign, EncounterId: z.enc.GetId(), PendingDamageId: pending[0].GetId(), IdempotencyKey: newKey(), Amount: &one,
	})))
	reads("with the damage taken")

	events := z.endAndCollect(watchers)
	for _, p := range []*person{ana, caio} {
		requireClean(t, p.name, "the stream", bytesOf(events[p]), needles)
	}
}

// conditionsOf is the conditions of the combatant, and their names, as the person's combat reads them.
func (z *zoneTable) conditionsOf(p *person, combatantID string) []string {
	for _, c := range must(p.combat.GetEncounter(z.t.Context(), rq(&playv1.GetEncounterRequest{CampaignId: z.campaign}))).GetEncounter().GetCombatants() {
		if c.GetId() == combatantID {
			return append(slices.Clone(c.GetConditions()), c.GetConditionNamesPt()...)
		}
	}
	z.t.Fatalf("%s does not read the combatant %s", p.name, combatantID)
	return nil
}

// A hidden caster's Silence deafens the creature inside: the master and that creature's player read the condition, another
// player does not, and none of them receives the spell, the caster or the zone. The deafened wizard's own player is the
// positive control, and so is the master's copy of the warlock outside the sphere: the sphere is what deafens.
func TestAHiddenCastersSilenceDeafensInTheMastersAndTheOwnersViewAlone(t *testing.T) {
	z := newZoneTableOf(t, wallCaster)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%v", r)
		}
	}()
	ana, caio, master := z.ana, z.caio, z.master
	watchers := z.streamsOf()
	z.untilTurn(z.casterC)
	// 20 feet around (3,3): the wizard at (3,5) is inside, the warlock at (3,9) and the creature at (9,5) are not.
	cast := z.cast(master, z.casterC, "spell:silence", 2, wizardCol, wizardRow-2)
	zones := cast.GetEncounter().GetZones()
	if len(zones) != 1 {
		t.Fatalf("the master's cast holds %d zones, want the sphere", len(zones))
	}
	z.zoneID = zones[0].GetId()
	needles := []string{z.casterName, z.caster.GetId(), z.casterC, z.zoneID, "spell:silence", "Silence", "Silêncio"}
	const deafened = "condition:deafened"

	if got := z.conditionsOf(master, z.wizardC); !slices.Contains(got, deafened) {
		t.Fatalf("the master reads the wizard's conditions %v, want deafened: the sphere did not deafen it", got)
	}
	if got := z.conditionsOf(master, z.warlockC); slices.Contains(got, deafened) {
		t.Errorf("the master reads the warlock outside the sphere deafened: %v", got)
	}
	if got := z.conditionsOf(ana, z.wizardC); !slices.Contains(got, deafened) {
		t.Errorf("Ana reads her wizard's conditions %v, want deafened: it is her own", got)
	}
	if got := z.conditionsOf(caio, z.wizardC); len(got) != 0 {
		t.Errorf("Caio reads Ana's wizard with the conditions %v, want none: the deafness has no sign outside", got)
	}
	for _, p := range []*person{ana, caio} {
		if zs := z.zonesOf(p); len(zs) != 0 {
			t.Errorf("%s reads the zones %v of a hidden caster", p.name, zs)
		}
		text := z.read(p)
		requireClean(t, p.name, "the map and the combat with the hidden caster's Silence", text, needles)
		requireClean(t, p.name, "the answer of GetMapVision", jsonOf(z.vision(p, z.fogMap)), needles)
	}
	if text := z.read(caio); strings.Contains(text, deafened) {
		t.Errorf("Caio's map and combat hold %q: another player's deafness", deafened)
	}
	requireHolds(t, master.name, "the master's combat with the Silence", z.read(master), []string{z.casterC, z.zoneID})

	events := z.endAndCollect(watchers)
	for _, p := range []*person{ana, caio} {
		requireClean(t, p.name, "the stream", bytesOf(events[p]), needles)
	}
	if strings.Contains(bytesOf(events[caio]), deafened) {
		t.Errorf("Caio's stream holds %q: another player's deafness", deafened)
	}
}

// A Spike Growth cast by a hidden NPC that then walks in it: no player receives the zone, the caster, the damage the walk
// left to roll, or the line of the move (zone_move), in the combat, the log, the map or the stream. The master's combat
// holds the zone, the caster and the damage: the positive control (the caster is the master's own creature).
func TestAHiddenCastersSpikeGrowthAndItsWalkReachNoPlayer(t *testing.T) {
	z := newZoneTableOf(t, spikeCaster)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%v", r)
		}
	}()
	ctx := t.Context()
	ana, caio, master := z.ana, z.caio, z.master
	watchers := z.streamsOf()

	z.untilTurn(z.casterC)
	cast := z.cast(master, z.casterC, "spell:spike-growth", 2, casterCol, casterRow)
	zones := cast.GetEncounter().GetZones()
	if len(zones) != 1 {
		t.Fatalf("the master's cast holds the zones %v, want the spikes", zones)
	}
	z.zoneID = zones[0].GetId()
	// The hidden caster walks two squares inside its own spikes, on its turn: the walk leaves damage to roll and a zone line.
	must(master.combat.MoveCombatant(ctx, rq(&playv1.MoveCombatantRequest{
		CampaignId: z.campaign, EncounterId: z.enc.GetId(), CombatantId: z.casterC, IdempotencyKey: newKey(), Col: casterCol - 2, Row: casterRow,
	})))
	needles := []string{z.casterName, z.caster.GetId(), z.casterC, z.zoneID}

	requireHolds(t, master.name, "the master's combat with the spikes and the walk", z.read(master), needles)
	if zs := z.zonesOf(master); len(zs) != 1 || zs[0].GetVisibility() != playv1.SpellZoneVisibility_SPELL_ZONE_VISIBILITY_MASTER_ONLY {
		t.Fatalf("the master reads zones %v, want the hidden caster's spikes marked for the master alone", zs)
	}
	for _, p := range []*person{ana, caio} {
		if zs := z.zonesOf(p); len(zs) != 0 {
			t.Errorf("%s reads the zones %v of a hidden caster", p.name, zs)
		}
		requireClean(t, p.name, "the map, the combat and its log with the hidden caster's spikes and walk", z.read(p), needles)
		if slices.Contains(z.tokensOf(p), z.caster.GetId()) || z.onTheMap(p, z.casterC) {
			t.Errorf("%s's map places the hidden caster", p.name)
		}
	}
	events := z.endAndCollect(watchers)
	for _, p := range []*person{ana, caio} {
		requireClean(t, p.name, "the stream", bytesOf(events[p]), needles)
	}
	requireHolds(t, master.name, "the master's stream", bytesOf(events[master]), []string{z.casterC}) // the stream names the combatant, not the name
}

// The master's calls ride a zone's notes (master_light, master_wind, master_breath) and belong to the master alone: the
// caster's own player, who reads the zone's numbers, and another player get no such note in the raw body of the combat.
func TestTheMastersCallsOfAZoneReachNoPlayer(t *testing.T) {
	for spell, call := range map[string]string{"spell:darkness": "master_light", "spell:fog-cloud": "master_wind"} {
		t.Run(spell, func(t *testing.T) {
			z := newZoneTable(t, false)
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%v", r)
				}
			}()
			z.untilTurn(z.wizardC)
			z.cast(z.ana, z.wizardC, spell, 2, foeCol+3, foeRow)

			// The master reads the call (the positive control); the caster's player and the other player never do.
			requireHolds(t, z.master.name, "the combat with the zone", z.read(z.master), []string{call})
			for _, p := range []*person{z.ana, z.caio} {
				body := z.read(p)
				requireClean(t, p.name, "the combat", body, []string{"master_"})
				if p == z.ana {
					requireHolds(t, p.name, "the caster's own combat", body, []string{spell})
				}
			}
		})
	}
}

// eastWall is the wall from its point to the south that burns the east side, the side behind the wall for the players.
func eastWall(r *playv1.CastSpellRequest) {
	r.Wall = &playv1.SpellWall{Direction: &playv1.SpellDirection{Dy: 1}, Side: playv1.WallSide_WALL_SIDE_LEFT}
}

// A revealed caster's Wall of Fire is read by a player where they see now (RN-10): the face of the wall that is turned to
// them and the burning squares in their sight, never the burning side behind the wall, which the fog hides. The master reads
// the whole wall and the burning side behind it: the positive control.
func TestAPlayerReadsOnlyTheSeenSquaresOfARevealedCastersWall(t *testing.T) {
	z := newZoneTableOf(t, wallCaster)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%v", r)
		}
	}()
	ctx := t.Context()
	z.untilTurn(z.casterC)
	z.cast(z.master, z.casterC, "spell:wall-of-fire", 4, wallCol, wallRow, eastWall)
	must(z.master.combat.SetCombatantHidden(ctx, rq(&playv1.SetCombatantHiddenRequest{
		CampaignId: z.campaign, EncounterId: z.enc.GetId(), CombatantId: z.casterC, IdempotencyKey: newKey(), Hidden: false,
	})))
	z.forceMove(z.casterC, wizardCol+1, wizardRow+2) // on the players' side of the wall, in their sight

	masterZones := z.zonesOf(z.master)
	if len(masterZones) != 1 || len(masterZones[0].GetBurningSquares()) == 0 {
		t.Fatalf("the master reads %v, want the wall with its burning squares", masterZones)
	}
	behind := func(sqs []*playv1.SpellOrigin) []*playv1.SpellOrigin {
		var out []*playv1.SpellOrigin
		for _, sq := range sqs {
			if sq.GetCol() > wallCol {
				out = append(out, sq)
			}
		}
		return out
	}
	if len(behind(masterZones[0].GetBurningSquares())) == 0 || masterZones[0].GetWallSide() != playv1.WallSide_WALL_SIDE_LEFT {
		t.Fatalf("the master's burning squares %v hold none behind the wall: the row proves nothing", masterZones[0].GetBurningSquares())
	}
	for _, p := range []*person{z.ana, z.caio} {
		view := z.vision(p, z.fogMap)
		for _, zn := range z.zonesOf(p) {
			if got := behind(zn.GetBurningSquares()); len(got) != 0 {
				t.Errorf("%s reads the burning squares %v behind the wall", p.name, got)
			}
			for _, sq := range append(zn.GetSquares(), zn.GetBurningSquares()...) {
				if sq.GetCol() > wallCol {
					t.Errorf("%s reads the square %v behind the wall\n%s", p.name, sq, strings.Join(picture(view), "\n"))
				}
			}
			// The answer holds nothing of the squares in the dark: its origin is one of the squares it reads.
			if !slices.ContainsFunc(zn.GetSquares(), func(sq *playv1.SpellOrigin) bool {
				return sq.GetCol() == zn.GetOrigin().GetCol() && sq.GetRow() == zn.GetOrigin().GetRow()
			}) {
				t.Errorf("%s reads the origin %v, which is none of the squares %v it reads", p.name, zn.GetOrigin(), zn.GetSquares())
			}
			for _, sq := range zn.GetBurningSquares() {
				if !seenNow(view, int(sq.GetCol()), int(sq.GetRow())) {
					t.Errorf("%s reads the burning square %v that they do not see now", p.name, sq)
				}
			}
		}
	}
}
