package leaktest

import (
	"testing"
	"time"
	"uuid"

	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	notesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/notes/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules/grid"
)

// newWorld builds the fixture: the master's campaign with one of every kind of
// hidden thing. It is long on purpose, and each step says what it hides and from
// whom. A step that fails stops the test with the line of the call.
func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{stack: newStack(t), secrets: newSecrets(), pts: map[string]*mapsv1.MapPoint{}, puzzles: map[string]*playv1.Puzzle{}, notes: map[string]string{}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("building the fixture: %v", r)
		}
	}()
	w.buildPeople()
	w.buildCharacters()
	w.buildGallery()
	w.buildFogMap()
	w.buildOtherMaps()
	w.buildTraps()
	w.buildSession()
	w.buildCombat()
	w.buildPuzzles()
	w.buildNotesAndProgress()
	w.buildItems()
	w.buildTableContent()
	w.buildGenerated()
	w.buildStreamTargets()
	return w
}

// buildPeople makes the master, two players, a pending member and a stranger.
func (w *world) buildPeople() {
	w.master, w.ana, w.caio = w.newPerson("Mestre"), w.newPerson("Ana"), w.newPerson("Caio")
	w.pending, w.stranger = w.newPerson("Bia"), w.newPerson("Eva")
	camp := must(w.master.campaigns.CreateCampaign(w.t.Context(), rq(&campaignsv1.CreateCampaignRequest{Name: "Mirathel", XpMode: campaignsv1.XpMode_XP_MODE_MILESTONES}))).GetCampaign()
	w.campaign = camp.GetId()
	w.join(false, w.ana, w.caio)
	w.join(true, w.pending)
	// A pending member may create the one character that waits for the master's approval.
	w.pendingHero = w.pc(w.pending, "Bia, a recém-chegada", "race:human")
}

// buildCharacters makes the players' characters and the NPCs, with the secret
// numbers on the NPC's sheet and the master's notes on the characters.
func (w *world) buildCharacters() {
	ctx := w.t.Context()
	w.pens = w.pc(w.ana, "Pensantus", "race:gnome")
	w.toren = w.pc(w.caio, "Toren", "race:human")
	// The master's private notes about a character (Ana's and the NPC's).
	for _, id := range []string{w.pens.GetId(), w.toren.GetId()} {
		must(w.master.characters.UpdateMasterNotes(ctx, rq(&charactersv1.UpdateMasterNotesRequest{CampaignId: w.campaign, CharacterId: id, Notes: w.secrets.marker("master-notes")})))
	}

	w.secrets.number("npc-hit-points", bossHP)
	w.secrets.number("npc-armor-class", bossAC, "armor_class")
	w.secrets.number("npc-xp", bossXP)
	var name string
	w.boss, name = w.npc("boss", &charactersv1.BasicSheet{
		HitPointsMax: bossHP, ArmorClass: bossAC, SpeedFt: 30, XpValue: bossXP, ChallengeRating: "3",
		Attacks: []*charactersv1.BasicAttack{{Name: w.secrets.marker("boss-attack-name"), AttackBonus: 6, DamageDiceCount: 2, DamageDiceSides: 8, DamageBonus: 3, DamageType: charactersv1.DamageType_DAMAGE_TYPE_SLASHING}},
	})
	_ = name
	w.secrets.id("boss", w.boss.GetId())
	must(w.master.characters.UpdateMasterNotes(ctx, rq(&charactersv1.UpdateMasterNotesRequest{CampaignId: w.campaign, CharacterId: w.boss.GetId(), Notes: w.secrets.marker("master-notes")})))

	// An NPC in plain sight of Ana's character: its name is hers to read, its numbers are not.
	w.secrets.number("npc-hit-points", 4324)
	w.seenNPC, _ = w.npc("seen-npc", &charactersv1.BasicSheet{HitPointsMax: 4324, ArmorClass: bossAC, SpeedFt: 30, XpValue: bossXP})
	w.allow("seen-npc-name", w.ana)
	w.allow("seen-npc-description")
	w.secrets.id("seen-npc", w.seenNPC.GetId()) // during a combat the NPC tokens are left out of the map reads: the id is the master's

	w.hiddenNPC, _ = w.npc("hidden-npc", &charactersv1.BasicSheet{HitPointsMax: 4322, ArmorClass: 12, SpeedFt: 30})
	w.secrets.number("npc-hit-points", 4322)
	w.secrets.id("hidden-npc", w.hiddenNPC.GetId())

	// An NPC made from a bestiary creature: its creature key and numbers are the master's.
	w.bandit = must(w.master.characters.CreateNpcFromCreature(ctx, rq(&charactersv1.CreateNpcFromCreatureRequest{
		CampaignId: w.campaign, CreatureKey: "monster:bandit", Name: w.secrets.marker("bandit-name"), Kind: charactersv1.CharacterKind_CHARACTER_KIND_MINION, IdempotencyKey: newKey(),
	}))).GetCharacter()
	w.secrets.id("bandit", w.bandit.GetId())
	w.secrets.add(&canary{needle: "monster:bandit", kind: "monster-key"})
}

// buildGallery uploads the pictures: each is named with a marker, which is the
// master's alone.
func (w *world) buildGallery() {
	w.imgMap = w.image("map-image", 240, 160)
	w.imgUnshown = w.image("unshown-image", 80, 60)
	// What the master shows is the players': the name goes with it.
	w.imgShown = w.image("shown-image", 90, 60, w.ana, w.caio)
	w.imgLeft = w.image("left-image", 100, 60, w.ana, w.caio)
	w.imgStage = w.image("stage-portrait", 60, 60) // its gallery name is the master's; players fetch the picture only
	w.imgPortrait = w.image("portrait-image", 60, 60)
	for _, id := range []string{w.imgMap, w.imgUnshown, w.imgPortrait} {
		w.secrets.id("gallery-image", id)
	}
}

// buildFogMap makes the map of the session: a cave with the fog on, in which Ana's
// character sees the west and Caio's the south room, and the guard room is seen
// by nobody. It is the map players read: the revealed things on it that nobody's
// character sees must not reach them either.
func (w *world) buildFogMap() {
	ctx := w.t.Context()
	m := w.master
	w.fogMap = must(m.maps.CreateMap(ctx, rq(&mapsv1.CreateMapRequest{CampaignId: w.campaign, Name: w.secrets.public("fog-map-name"), ImageId: w.imgMap}))).GetMap().GetId()
	must(m.maps.SetMapRevealed(ctx, rq(&mapsv1.SetMapRevealedRequest{CampaignId: w.campaign, MapId: w.fogMap, Revealed: true})))
	must(m.maps.SetMapGrid(ctx, rq(&mapsv1.SetMapGridRequest{CampaignId: w.campaign, MapId: w.fogMap, Columns: caveColumns})))

	var walls, rubble, crates [][2]int32
	for row, line := range caveWalls {
		for col, ch := range line {
			switch ch {
			case '#':
				walls = append(walls, [2]int32{int32(col), int32(row)}) //nolint:gosec // G115: a cave of 24 x 16
			case ':':
				rubble = append(rubble, [2]int32{int32(col), int32(row)}) //nolint:gosec // G115: a cave of 24 x 16
			case 'h':
				crates = append(crates, [2]int32{int32(col), int32(row)}) //nolint:gosec // G115: a cave of 24 x 16
			}
		}
	}
	w.paint(w.fogMap, mapsv1.MapLayer_MAP_LAYER_WALL, 1, walls)
	w.paint(w.fogMap, mapsv1.MapLayer_MAP_LAYER_DIFFICULT_TERRAIN, 1, rubble)
	w.paint(w.fogMap, mapsv1.MapLayer_MAP_LAYER_COVER, 1, crates)
	// The master's painted light is never a player's.
	w.paint(w.fogMap, mapsv1.MapLayer_MAP_LAYER_LIGHT, 3, [][2]int32{{17, 3}, {18, 3}, {17, 4}, {18, 4}})
	must(m.maps.SetMapFog(ctx, rq(&mapsv1.SetMapFogRequest{CampaignId: w.campaign, MapId: w.fogMap, FogEnabled: new(true), BaseLight: mapsv1.LightLevel_LIGHT_LEVEL_BRIGHT.Enum()})))

	// What Ana's character sees (the west), the master made public.
	w.pts["entrance"] = w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, w.secrets.marker("entrance-name", w.ana), w.secrets.marker("entrance-description", w.ana), 2, 7, nil)
	w.reveal(w.pts["entrance"])
	w.pts["west-treasure"] = w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE, w.secrets.marker("treasure-revealed-name", w.ana), w.secrets.marker("treasure-revealed-description"), 2, 9, func(r *mapsv1.CreateMapPointRequest) {
		r.TreasureValuePo = new(w.value(treasurePO))
	})
	w.reveal(w.pts["west-treasure"])

	// What only Caio's character sees (the south room): the same, for him.
	w.pts["south-treasure"] = w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE, w.secrets.marker("south-treasure-name", w.caio), w.secrets.marker("south-treasure-description"), 10, 12, func(r *mapsv1.CreateMapPointRequest) {
		r.TreasureValuePo = new(w.value(treasurePO))
	})
	w.reveal(w.pts["south-treasure"])

	// Revealed, but in the guard room, which no character sees: the fog keeps them.
	w.pts["guardhouse"] = w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, w.secrets.marker("fog-point-name"), w.secrets.marker("fog-point-description"), 19, 3, nil)
	w.reveal(w.pts["guardhouse"])
	w.pts["fog-treasure"] = w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE, w.secrets.marker("fog-treasure-name"), w.secrets.marker("fog-treasure-description"), 20, 4, func(r *mapsv1.CreateMapPointRequest) {
		r.TreasureValuePo = new(w.value(treasurePO))
	})
	w.reveal(w.pts["fog-treasure"])

	// Characters' tokens: Ana's gnome sees the west (and along the corridor), Caio's
	// human the south room; a goblin in plain sight, the boss in the guard room (revealed
	// by the master, seen by nobody) and a goblin the master hid.
	w.place(w.fogMap, w.pens.GetId(), 5, 8)
	w.place(w.fogMap, w.toren.GetId(), 10, 13)
	w.place(w.fogMap, w.boss.GetId(), 21, 3)
	w.showToken(w.fogMap, w.boss.GetId())
	w.place(w.fogMap, w.hiddenNPC.GetId(), 20, 8)
	w.place(w.fogMap, w.bandit.GetId(), 22, 2)
	w.place(w.fogMap, w.seenNPC.GetId(), 6, 7) // a goblin in Ana's sight (not Caio's), left visible
	w.showToken(w.fogMap, w.seenNPC.GetId())
}

func (w *world) showToken(mapID, characterID string) {
	must(w.master.maps.SetMapTokenHidden(w.t.Context(), rq(&mapsv1.SetMapTokenHiddenRequest{CampaignId: w.campaign, MapId: mapID, CharacterId: characterID, Hidden: false})))
}

// value registers a secret number and returns it as int32 for a request.
func (w *world) value(n int64) int32 {
	w.secrets.number("treasure-value-po", n)
	return int32(n) //nolint:gosec // G115: a small constant
}

// buildOtherMaps makes the maps a player never reads: a hidden map with points,
// and one that leads to it through a revealed submap point.
func (w *world) buildOtherMaps() {
	ctx := w.t.Context()
	m := w.master
	img := w.image("hidden-map-image", 120, 80)
	w.imgHiddenMap = img
	w.secrets.id("gallery-image", img)
	w.hiddenMap = must(m.maps.CreateMap(ctx, rq(&mapsv1.CreateMapRequest{CampaignId: w.campaign, Name: w.secrets.marker("hidden-map-name"), ImageId: img}))).GetMap().GetId()
	w.secrets.id("hidden-map", w.hiddenMap)
	w.pts["hidden-map-point"] = w.point(w.hiddenMap, mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, w.secrets.marker("hidden-map-point-name"), w.secrets.marker("hidden-map-point-description"), 4, 4, nil)
	// A revealed submap point of the cave that leads to the hidden map.
	w.pts["submap"] = w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_SUBMAP, w.secrets.marker("submap-name", w.ana), w.secrets.marker("submap-description", w.ana), 4, 7, func(r *mapsv1.CreateMapPointRequest) { r.TargetMapId = w.hiddenMap })
	w.reveal(w.pts["submap"])
}

func newKey() string { return uuid.New().String() }

var (
	_ = grid.Square{}
	_ = rulesv1.TrapTrigger_TRAP_TRIGGER_ENTER
)

// buildTraps makes the traps: one nobody knows, one only Ana's character knows, one
// the master revealed to everyone (a point of the fog map, so only who sees its squares
// gets it). Whatever a player knows of a trap, the DCs and the effect are the master's.
func (w *world) buildTraps() {
	ctx := w.t.Context()
	w.secrets.number("trap-notice-dc", trapNotice, "dc")
	w.secrets.number("trap-find-dc", trapFind, "dc")
	w.secrets.number("trap-save-dc", trapSave, "dc")
	spec := w.trapSpec
	trap := func(role, name, desc string, col, row int, readers ...*person) {
		w.pts[role] = w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, w.secrets.marker(name, readers...), w.secrets.marker(desc, readers...), col, row, func(r *mapsv1.CreateMapPointRequest) { r.Trap = spec() })
		w.secrets.id("trap", w.pts[role].GetId(), readers...)
	}
	trap("trap-hidden", "trap-hidden-name", "trap-hidden-description", 7, 8)
	trap("trap-known-ana", "trap-ana-name", "trap-ana-description", 4, 8, w.ana)
	must(w.master.maps.RevealTrap(ctx, rq(&mapsv1.RevealTrapRequest{CampaignId: w.campaign, MapId: w.fogMap, PointId: w.pts["trap-known-ana"].GetId(), CharacterIds: []string{w.pens.GetId()}})))
	// A trap the master never revealed, in the guard room, where nobody looks.
	trap("trap-guardroom", "trap-guardroom-name", "trap-guardroom-description", 18, 6)

	// A hidden treasure and a light, which a player never receives.
	w.pts["treasure-hidden"] = w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_TREASURE, w.secrets.marker("treasure-hidden-name"), w.secrets.marker("treasure-hidden-description"), 12, 13, func(r *mapsv1.CreateMapPointRequest) {
		r.TreasureValuePo = new(w.value(treasurePO))
	})
	w.secrets.id("treasure", w.pts["treasure-hidden"].GetId())
	w.pts["light"] = w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_LIGHT, w.secrets.marker("light-name"), w.secrets.marker("light-description"), 19, 4, func(r *mapsv1.CreateMapPointRequest) {
		r.Light = &mapsv1.LightSpec{PresetKey: "light:torch"}
	})
	w.secrets.id("light", w.pts["light"].GetId())
}

// buildSession opens the session and puts things in front of the players: the
// current map, a shown image, a left image, an open scene with a stage, and the
// scenes the master prepared and never opened.
func (w *world) buildSession() {
	ctx := w.t.Context()
	m := w.master
	w.session = must(m.play.StartGameSession(ctx, rq(&playv1.StartGameSessionRequest{CampaignId: w.campaign}))).GetGameSession().GetId()
	must(m.play.SetCurrentMap(ctx, rq(&playv1.SetCurrentMapRequest{CampaignId: w.campaign, MapId: w.fogMap})))

	// Left with the players, then another shown.
	must(m.play.SetShownImage(ctx, rq(&playv1.SetShownImageRequest{CampaignId: w.campaign, ImageId: w.imgLeft, Keep: true})))
	must(m.play.SetShownImage(ctx, rq(&playv1.SetShownImageRequest{CampaignId: w.campaign, ImageId: w.imgShown})))

	// The scene the master opens: it is a hidden point, and what the master opened is
	// what the players read (name, description, the checks), but not the hooks, the
	// DCs nor the clues that were not revealed to them.
	w.secrets.number("scene-dc", sceneDC, "dc")
	s1 := w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, w.secrets.public("scene-open-name"), w.secrets.public("scene-open-description"), 1, 8, func(r *mapsv1.CreateMapPointRequest) {
		r.Hooks = w.secrets.marker("scene-open-hooks")
	})
	w.pts["scene-open"] = s1
	w.secrets.id("scene-open-point", s1.GetId(), w.ana, w.caio) // the open scene names its point
	w.addAction(s1, "skill:perception", w.secrets.public("scene-open-action"), sceneDC, 5)
	clueAna := w.addClue(s1, w.secrets.marker("clue-ana", w.ana))
	w.clueUnrevealed = w.addClue(s1, w.secrets.marker("clue-unrevealed"))
	must(m.maps.RevealSceneClue(ctx, rq(&mapsv1.RevealSceneClueRequest{CampaignId: w.campaign, ClueId: clueAna, CharacterIds: []string{w.pens.GetId()}})))
	must(m.play.OpenScene(ctx, rq(&playv1.OpenSceneRequest{CampaignId: w.campaign, PointId: s1.GetId()})))
	// The stage: a merchant in, an NPC with a portrait out.
	w.merchant, _ = w.npc("merchant", &charactersv1.BasicSheet{HitPointsMax: 9, ArmorClass: 11, SpeedFt: 30, PortraitImageId: w.imgStage})
	// A stage NPC is public; its marker name was registered as secret by npc(), so allow it.
	w.allow("merchant-name", w.ana, w.caio)
	w.allow("merchant-description")
	w.offstage, _ = w.npc("offstage", &charactersv1.BasicSheet{HitPointsMax: 9, ArmorClass: 11, SpeedFt: 30, PortraitImageId: w.imgPortrait})
	w.secrets.id("offstage-npc", w.offstage.GetId())
	must(m.play.PutOnStage(ctx, rq(&playv1.PutOnStageRequest{CampaignId: w.campaign, CharacterId: w.merchant.GetId()})))

	// A scene prepared and never opened: everything on it is the master's.
	s2 := w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_SCENE, w.secrets.marker("scene-closed-name"), w.secrets.marker("scene-closed-description"), 6, 8, func(r *mapsv1.CreateMapPointRequest) {
		r.Hooks = w.secrets.marker("scene-closed-hooks")
	})
	w.pts["scene-closed"] = s2
	w.secrets.id("scene-closed-point", s2.GetId())
	w.addAction(s2, "skill:insight", w.secrets.marker("scene-closed-action"), sceneDC, 1)
	w.addClue(s2, w.secrets.marker("clue-closed"))
}

func (w *world) addAction(p *mapsv1.MapPoint, key, name string, dc, attempts int32) {
	must(w.master.maps.AddSceneAction(w.t.Context(), rq(&mapsv1.AddSceneActionRequest{CampaignId: w.campaign, MapId: p.GetMapId(), PointId: p.GetId(), Key: key, Name: name, Dc: dc, MaxAttempts: new(attempts)})))
}

func (w *world) addClue(p *mapsv1.MapPoint, text string) string {
	c := must(w.master.maps.AddSceneClue(w.t.Context(), rq(&mapsv1.AddSceneClueRequest{CampaignId: w.campaign, MapId: p.GetMapId(), PointId: p.GetId(), Text: text})))
	id := c.GetClue().GetId()
	w.secrets.id("clue", id)
	return id
}

// allow makes a canary the readers' to read: used when the master shows what an
// helper registered as secret.
func (w *world) allow(kind string, readers ...*person) {
	for _, c := range w.secrets.list {
		if c.kind == kind {
			c.readers = names(readers)
		}
	}
}

// buildCombat starts a combat on the fog map: the boss (revealed, in the guard room,
// seen by nobody), an NPC and a bestiary NPC the master left hidden, monsters added
// by creature key, and the master's battle point. Ana's character falls to 0 hit
// points and rolls a death save, which the table keeps to its owner and the master.
func (w *world) buildCombat() {
	ctx := w.t.Context()
	m := w.master
	// The table keeps the death saves to the owner and the master (RN-24).
	rules := must(m.campaigns.GetTableRules(ctx, rq(&campaignsv1.GetTableRulesRequest{CampaignId: w.campaign}))).GetRules()
	rules.DeathSaves = campaignsv1.DeathSaveVisibility_DEATH_SAVE_VISIBILITY_OWNER_AND_MASTER
	must(m.campaigns.SetTableRules(ctx, rq(&campaignsv1.SetTableRulesRequest{CampaignId: w.campaign, Rules: rules})))

	// The battle point the combat starts from, with its stored encounter.
	w.pts["battle"] = w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_BATTLE, w.secrets.marker("battle-name"), w.secrets.marker("battle-description"), 21, 8, nil)
	w.secrets.id("battle-point", w.pts["battle"].GetId())
	must(m.encounters.SaveBattleEncounter(ctx, rq(&playv1.SaveBattleEncounterRequest{
		CampaignId: w.campaign, MapPointId: w.pts["battle"].GetId(),
		Encounter: &playv1.BattleEncounter{Monsters: []*playv1.MonsterGroup{{CreatureKey: "monster:ogre", Count: 1, Name: w.secrets.marker("battle-mon")}}},
	})))

	revealed := false
	e := must(m.combat.StartEncounter(ctx, rq(&playv1.StartEncounterRequest{
		CampaignId: w.campaign, IdempotencyKey: newKey(), Name: w.secrets.public("encounter-name"), MapPointId: w.pts["battle"].GetId(),
		Participants: []*playv1.Participant{
			{CharacterId: w.boss.GetId(), Hidden: &revealed},
			{CharacterId: w.seenNPC.GetId(), Hidden: &revealed},
			{CharacterId: w.hiddenNPC.GetId()},
			{CharacterId: w.bandit.GetId()},
		},
		Monsters: []*playv1.MonsterGroup{{CreatureKey: "monster:goblin", Count: 2, Name: w.secrets.marker("mon-group")}},
	}))).GetEncounter()
	w.secrets.add(&canary{needle: "monster:goblin", kind: "monster-key"})
	for _, c := range e.GetCombatants() {
		if c.GetKind() == playv1.CombatantKind_COMBATANT_KIND_PLAYER && !c.GetMine() {
			must(m.combat.SubmitInitiative(ctx, rq(&playv1.SubmitInitiativeRequest{
				CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: c.GetId(), IdempotencyKey: newKey(),
				Roll: &playv1.SubmitInitiativeRequest_D20Face{D20Face: 12},
			})))
		}
	}
	e = must(m.combat.BeginCombat(ctx, rq(&playv1.BeginCombatRequest{CampaignId: w.campaign, EncounterId: e.GetId(), IdempotencyKey: newKey()}))).GetEncounter()
	w.encounter = e
	for _, c := range e.GetCombatants() {
		w.secrets.id("combatant", c.GetId(), w.combatantReaders(c)...)
	}

	// Pensantus falls, and Ana rolls a death save on her turn: a failure.
	pens := w.combatantOf(e, w.pens.GetId())
	must(m.play.AdjustCharacterVitals(ctx, rq(&playv1.AdjustCharacterVitalsRequest{
		CampaignId: w.campaign, CharacterId: w.pens.GetId(), IdempotencyKey: newKey(), HitPointsCurrent: proto.Int32(0),
	})))
	for range 60 {
		cur := must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
		if w.combatantOf(cur, w.pens.GetId()).GetDeathSaveDue() { // her turn has come, and she is down: the save is due
			break
		}
		must(m.combat.EndTurn(ctx, rq(&playv1.EndTurnRequest{CampaignId: w.campaign, EncounterId: e.GetId(), IdempotencyKey: newKey(), ExpectedCombatantId: cur.GetCurrentCombatantId(), ExpectedRound: cur.GetRound()})))
	}
	must(w.ana.combat.RollDeathSave(ctx, rq(&playv1.RollDeathSaveRequest{
		CampaignId: w.campaign, EncounterId: e.GetId(), CombatantId: pens.GetId(), IdempotencyKey: newKey(),
		Roll: &playv1.RollDeathSaveRequest_D20Face{D20Face: 5},
	})))
	w.encounter = must(m.combat.GetEncounter(ctx, rq(&playv1.GetEncounterRequest{CampaignId: w.campaign}))).GetEncounter()
}

// combatantOf finds the combatant of a character in the master's copy of a combat.
func (w *world) combatantOf(e *playv1.Encounter, characterID string) *playv1.Combatant {
	for _, c := range e.GetCombatants() {
		if c.GetCharacterId() == characterID {
			return c
		}
	}
	panic("no combatant for the character " + characterID)
}

// combatantReaders are the people who may read the id of a player's combatant: the party's.
func (w *world) combatantReaders(c *playv1.Combatant) []*person {
	if c.GetKind() == playv1.CombatantKind_COMBATANT_KIND_PLAYER {
		return []*person{w.ana, w.caio}
	}
	if c.GetCharacterId() == w.seenNPC.GetId() { // the goblin in Ana's sight
		return []*person{w.ana}
	}
	return nil
}

// buildPuzzles makes the puzzles: a lock the players are solving (its solution, its
// unreleased hints, the DC of a hint and what it does when solved are the master's),
// a riddle and a sequence never shown, an archived cipher, and a lock whose clue is
// split: each player reads their own part and never another's.
func (w *world) buildPuzzles() {
	ctx := w.t.Context()
	m := w.master
	w.secrets.number("hint-dc", hintDC, "dc")
	create := func(role string, req *playv1.CreatePuzzleRequest) *playv1.Puzzle {
		req.CampaignId = w.campaign
		pz := must(m.puzzles.CreatePuzzle(ctx, rq(req))).GetPuzzle()
		w.puzzles[role] = pz
		// the id of a puzzle on screen is the table's; any other is the master's
		if role == "shown" || role == "riddle-shown" || role == "split" {
			w.secrets.id("puzzle", pz.GetId(), w.ana, w.caio)
		} else {
			w.secrets.id("puzzle", pz.GetId())
		}
		return pz
	}
	lock := func(wheels int32) *playv1.PuzzleConfig {
		return &playv1.PuzzleConfig{Kind: &playv1.PuzzleConfig_Lock{Lock: &playv1.LockConfig{Wheels: wheels, Alphabet: playv1.PuzzleAlphabet_PUZZLE_ALPHABET_DIGITS}}}
	}
	lockSolution := &playv1.PuzzleSolution{Kind: &playv1.PuzzleSolution_Lock{Lock: &playv1.LockSolution{Wheels: []int32{7, 3, 5, 1}}}}
	lockStart := &playv1.PuzzleState{Kind: &playv1.PuzzleState_Lock{Lock: &playv1.LockState{Wheels: []int32{0, 0, 0, 0}}}}

	// The lock on screen. What the master told it to do is the master's: where it
	// leads (a point of the hidden map), the trap a wrong move fires, the DC of the hint.
	shown := create("shown", &playv1.CreatePuzzleRequest{
		Name: w.secrets.marker("puzzle-shown-name", w.ana, w.caio), Config: lock(4), Solution: lockSolution, Start: lockStart,
		Clue:  w.secrets.marker("puzzle-shown-clue", w.ana, w.caio),
		Hints: []string{w.secrets.marker("puzzle-hint"), w.secrets.marker("puzzle-hint"), w.secrets.marker("puzzle-hint")},
		OnSolve: &playv1.PuzzleOnSolve{
			Action: playv1.PuzzleSolveAction_PUZZLE_SOLVE_ACTION_REVEAL_POINT, Message: w.secrets.marker("puzzle-solved-message"), // read by the players once they solve it: they never do here
			Target: &playv1.PuzzleOnSolve_Point{Point: &playv1.PuzzlePointTarget{MapId: w.hiddenMap, PointId: w.pts["hidden-map-point"].GetId()}},
		},
		OnWrong: &playv1.PuzzleOnWrong{MaxMoves: 60},
	})
	must(m.puzzles.ShowPuzzle(ctx, rq(&playv1.ShowPuzzleRequest{CampaignId: w.campaign, PuzzleId: shown.GetId()})))
	must(m.puzzles.ReleaseNextPuzzleHint(ctx, rq(&playv1.ReleaseNextPuzzleHintRequest{CampaignId: w.campaign, PuzzleId: shown.GetId()})))
	// the first hint is now released: a player reads it
	w.secrets.release("puzzle-hint-1", w.ana, w.caio)

	// The riddle on screen: a hint won by a skill check (its DC is the master's), and a wrong
	// answer fires a hidden trap (which trap is the master's).
	riddle := create("riddle-shown", &playv1.CreatePuzzleRequest{
		Name:      w.secrets.marker("puzzle-rshown-name", w.ana, w.caio),
		Config:    &playv1.PuzzleConfig{Kind: &playv1.PuzzleConfig_Riddle{Riddle: &playv1.RiddleConfig{Text: w.secrets.marker("puzzle-rshown-text", w.ana, w.caio)}}},
		Solution:  &playv1.PuzzleSolution{Kind: &playv1.PuzzleSolution_Riddle{Riddle: &playv1.RiddleSolution{Answers: []string{w.secrets.marker("puzzle-rshown-answer")}}}},
		Clue:      w.secrets.marker("puzzle-rshown-clue", w.ana, w.caio),
		Hints:     []string{w.secrets.marker("puzzle-rhint")},
		HintCheck: &playv1.PuzzleHintCheck{SkillKey: "skill:investigation", Dc: hintDC},
		OnWrong:   &playv1.PuzzleOnWrong{Trap: &playv1.PuzzleTrapTarget{MapId: w.fogMap, PointId: w.pts["trap-hidden"].GetId()}, AttemptsPerPlayer: 3},
	})
	must(m.puzzles.ShowPuzzle(ctx, rq(&playv1.ShowPuzzleRequest{CampaignId: w.campaign, PuzzleId: riddle.GetId()})))

	// A riddle and a sequence the master prepared and never showed.
	create("riddle-hidden", &playv1.CreatePuzzleRequest{
		Name:     w.secrets.marker("puzzle-riddle-name"),
		Config:   &playv1.PuzzleConfig{Kind: &playv1.PuzzleConfig_Riddle{Riddle: &playv1.RiddleConfig{Text: w.secrets.marker("puzzle-riddle-text")}}},
		Solution: &playv1.PuzzleSolution{Kind: &playv1.PuzzleSolution_Riddle{Riddle: &playv1.RiddleSolution{Answers: []string{w.secrets.marker("puzzle-riddle-answer")}}}},
		Clue:     w.secrets.marker("puzzle-riddle-clue"),
	})
	create("sequence", &playv1.CreatePuzzleRequest{
		Name:     w.secrets.marker("puzzle-sequence-name"),
		Config:   &playv1.PuzzleConfig{Kind: &playv1.PuzzleConfig_Sequence{Sequence: &playv1.SequenceConfig{Bells: 4}}},
		Solution: &playv1.PuzzleSolution{Kind: &playv1.PuzzleSolution_Sequence{Sequence: &playv1.SequenceSolution{Steps: []int32{2, 0, 3, 1}}}},
		Clue:     w.secrets.marker("puzzle-sequence-clue"),
	})

	// A cipher, archived: the master put it away, so it is no one's.
	cipher := create("cipher", &playv1.CreatePuzzleRequest{
		Name:     w.secrets.marker("puzzle-cipher-name"),
		Config:   &playv1.PuzzleConfig{Kind: &playv1.PuzzleConfig_Cipher{Cipher: &playv1.CipherConfig{}}},
		Solution: &playv1.PuzzleSolution{Kind: &playv1.PuzzleSolution_Cipher{Cipher: &playv1.CipherSolution{Message: "LEAKCANARY segredo da cifra", Method: &playv1.CipherSolution_Shift{Shift: 3}}}},
		Clue:     w.secrets.marker("puzzle-cipher-clue"),
	})
	w.secrets.add(&canary{needle: "LEAKCANARY segredo da cifra", kind: "puzzle-cipher-plain"})
	must(m.puzzles.ArchivePuzzle(ctx, rq(&playv1.ArchivePuzzleRequest{CampaignId: w.campaign, PuzzleId: cipher.GetId()})))

	// The split clue: Ana reads her part and Caio his, never each other's.
	split := create("split", &playv1.CreatePuzzleRequest{
		Name: w.secrets.marker("puzzle-split-name", w.ana, w.caio), Config: lock(3), Start: &playv1.PuzzleState{Kind: &playv1.PuzzleState_Lock{Lock: &playv1.LockState{Wheels: []int32{0, 0, 0}}}},
		Solution: &playv1.PuzzleSolution{Kind: &playv1.PuzzleSolution_Lock{Lock: &playv1.LockSolution{Wheels: []int32{4, 8, 2}}}},
		Clue:     w.secrets.marker("puzzle-split-clue", w.ana, w.caio),
		Parts: []*playv1.PuzzlePart{
			{CharacterId: w.pens.GetId(), Text: w.secrets.marker("puzzle-part-ana", w.ana)},
			{CharacterId: w.toren.GetId(), Text: w.secrets.marker("puzzle-part-caio", w.caio)},
		},
	})
	must(m.puzzles.ShowPuzzle(ctx, rq(&playv1.ShowPuzzleRequest{CampaignId: w.campaign, PuzzleId: split.GetId()})))
}

// buildNotesAndProgress makes what is written for or about one player: each one's
// private notes, a creature one was given, the master's document, the milestones and the XP.
func (w *world) buildNotesAndProgress() {
	ctx := w.t.Context()
	m := w.master
	for _, p := range []*person{w.ana, w.caio} {
		n := must(p.notes.CreateNote(ctx, rq(&notesv1.CreateNoteRequest{CampaignId: w.campaign, Text: w.secrets.marker("note-"+p.name, p), ScenePointId: w.pts["scene-open"].GetId()}))).GetNote()
		w.notes[p.name] = n.GetId()
		w.secrets.id("note", n.GetId(), p)
	}
	// A creature the master gave Ana's character: its numbers are hers and the master's.
	cr := must(m.characters.GiveCreature(ctx, rq(&charactersv1.GiveCreatureRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId(), MonsterKey: "monster:wolf", Name: w.secrets.marker("creature-ana", w.ana)}))).GetCreature()
	w.secrets.id("creature", cr.GetId(), w.ana)
	w.creature = cr.GetId()

	must(m.document.UpdateCampaignDocument(ctx, rq(&campaignsv1.UpdateCampaignDocumentRequest{CampaignId: w.campaign, Body: "# Segredos\n\n" + w.secrets.marker("campaign-document") + "\n"})))

	// Milestones: the master plans four; one is reached, and its text is the table's.
	planned := must(m.xp.AddMilestone(ctx, rq(&progressionv1.AddMilestoneRequest{CampaignId: w.campaign, Text: w.secrets.marker("milestone-unreached")})))
	_ = planned
	reached := must(m.xp.AddMilestone(ctx, rq(&progressionv1.AddMilestoneRequest{CampaignId: w.campaign, Text: w.secrets.marker("milestone-reached", w.ana, w.caio)}))).GetMilestone()
	must(m.xp.MarkMilestoneReached(ctx, rq(&progressionv1.MarkMilestoneReachedRequest{CampaignId: w.campaign, MilestoneId: reached.GetId(), CharacterIds: []string{w.pens.GetId(), w.toren.GetId()}, IdempotencyKey: newKey()})))
	w.secrets.id("milestone", reached.GetId(), w.ana, w.caio)
	w.secrets.id("milestone", planned.GetMilestone().GetId())
	w.milestoneUnreached = planned.GetMilestone().GetId()
}

// buildTableContent writes the table's own content and switches options off. What is
// archived or off is no one's but the master's (RN-23): a player is never offered it, and
// never reads its name nor its key.
func (w *world) buildTableContent() {
	ctx := w.t.Context()
	m := w.master
	create := func(kind string, body proto.Message, readers ...*person) *rulesv1.TableEntry {
		var req *rulesv1.CreateTableEntryRequest
		switch b := body.(type) {
		case *rulesv1.TableRace:
			b.NamePt = w.secrets.marker(kind, readers...)
			req = &rulesv1.CreateTableEntryRequest{CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableRace{TableRace: b}}
		case *rulesv1.TableSpell:
			b.NamePt = w.secrets.marker(kind, readers...)
			req = &rulesv1.CreateTableEntryRequest{CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableSpell{TableSpell: b}}
		case *rulesv1.TableBackground:
			b.NamePt = w.secrets.marker(kind, readers...)
			req = &rulesv1.CreateTableEntryRequest{CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableBackground{TableBackground: b}}
		}
		e := must(m.table.CreateTableEntry(ctx, rq(req))).GetEntry()
		w.secrets.id(kind+"-key", e.GetKey(), readers...)
		return e
	}
	race := func() *rulesv1.TableRace {
		return &rulesv1.TableRace{Size: "Medium", SpeedFt: 25, AbilityBonuses: &rulesv1.AbilityScores{Constitution: 1}, ChoiceBonuses: []int32{2, 1}, DarkvisionFt: 60, Languages: []string{"language:common"}, LanguageChoices: 1}
	}
	spell := func(readers ...*person) *rulesv1.TableSpell {
		return &rulesv1.TableSpell{
			Level: 1, SchoolKey: "school:evocation",
			CastingTime: &rulesv1.TableSpellCastingTime{Unit: rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_ACTION, Amount: 1},
			Range:       &rulesv1.TableSpellRange{Kind: rulesv1.SpellRangeKind_SPELL_RANGE_KIND_RANGED, DistanceFt: 60},
			Duration:    &rulesv1.TableSpellDuration{Kind: rulesv1.SpellDurationKind_SPELL_DURATION_KIND_INSTANTANEOUS},
			Components:  &rulesv1.TableSpellComponents{Verbal: true, Somatic: true},
			ClassKeys:   []string{"class:wizard"}, DescPt: []string{w.secrets.marker("spell-text", readers...)},
			Target: &rulesv1.TableSpellTarget{Kind: rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_CREATURE},
			Attack: "ranged",
			Damage: []*rulesv1.TableSpellDamage{{DamageTypeKey: "damage-type:force", Dice: "3d6"}},
		}
	}
	// one live entry, which the players read, so the reads are not vacuous
	create("race-live", race(), w.ana, w.caio, w.pending)
	create("spell-live", spell(w.ana, w.caio, w.pending), w.ana, w.caio, w.pending)
	archivedRace := create("race-archived", race())
	must(m.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: archivedRace.GetKey()})))
	archivedSpell := create("spell-archived", spell())
	must(m.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: archivedSpell.GetKey()})))
	offRace := create("race-off", race())
	// and an option of the SRD, switched off in "Opções para os jogadores"
	must(m.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{
		{Key: offRace.GetKey(), Off: true}, {Key: "race:tiefling", Off: true},
	}})))
	w.secrets.add(&canary{needle: "race:tiefling", kind: "srd-race-off-key"})
}

// buildGenerated makes what the tools made and the master has not shown: an image the
// generator made, a hidden dungeon map and its rooms, a treasure placed on the map.
func (w *world) buildGenerated() {
	ctx := w.t.Context()
	m := w.master
	// A generated picture: it is in the gallery, and nobody has been shown it.
	gen := must(m.imagegen.GenerateSceneImage(ctx, rq(&mapsv1.GenerateSceneImageRequest{
		CampaignId: w.campaign, IdempotencyKey: newKey(), Prompt: w.secrets.marker("generated-prompt"), Name: w.secrets.marker("generated-name"),
		AspectRatio: mapsv1.ImageAspectRatio_IMAGE_ASPECT_RATIO_1_1,
	}))).GetGeneration()
	w.secrets.id("generation", gen.GetId())
	w.generation = gen.GetId()
	for range 50 { // the fake generator answers at once, but it works in the background
		g := must(m.imagegen.GetImageGeneration(ctx, rq(&mapsv1.GetImageGenerationRequest{CampaignId: w.campaign, GenerationId: gen.GetId(), WaitSeconds: 5})))
		if g.GetGeneration().GetState() == mapsv1.ImageGenerationState_IMAGE_GENERATION_STATE_DONE {
			w.secrets.id("generated-image", g.GetGeneration().GetImageId())
			w.imgGenerated = g.GetGeneration().GetImageId()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if w.imgGenerated == "" {
		panic("the generated image never came")
	}

	// A dungeon the master generated: a hidden map with a seed.
	w.secrets.number("dungeon-seed", dungeonSeed)
	seed := uint64(dungeonSeed)
	d := must(m.dungeons.CreateDungeonMap(ctx, rq(&mapsv1.CreateDungeonMapRequest{CampaignId: w.campaign, Name: w.secrets.marker("dungeon-name"), Seed: &seed}))).GetMap()
	w.dungeonMap = d.GetId()
	w.secrets.id("dungeon-map", w.dungeonMap)
	w.secrets.id("gallery-image", d.GetImage().GetId())
	w.imgDungeon = d.GetImage().GetId()

	// A treasure the generator rolled and the master placed on the cave: hidden.
	tseed := uint64(treasureSeed)
	w.secrets.number("treasure-seed", treasureSeed)
	rolled := must(m.treasure.GenerateTreasure(ctx, rq(&mapsv1.GenerateTreasureRequest{CampaignId: w.campaign, Mode: mapsv1.TreasureMode_TREASURE_MODE_HOARD, PartyLevel: proto.Int32(5), Seed: &tseed}))).GetTreasure()
	placed := must(m.treasure.PlaceTreasure(ctx, rq(&mapsv1.PlaceTreasureRequest{
		CampaignId: w.campaign, MapId: w.fogMap, Mode: mapsv1.TreasureMode_TREASURE_MODE_HOARD, PartyLevel: 5, Seed: &tseed, Column: 12, Row: 12,
		Name: new(w.secrets.marker("placed-treasure-name")), IdempotencyKey: newKey(), ContentVersion: rolled.GetContentVersion(),
	})))
	if p := placed.GetPoint(); p != nil {
		w.pts["placed-treasure"] = p
		w.secrets.id("treasure", p.GetId())
	}
}

// trapSpec is a trap with the secret DCs of the fixture: to notice, to find, and the save.
func (w *world) trapSpec() *mapsv1.TrapSpec {
	return &mapsv1.TrapSpec{
		NoticeDc: trapNotice, FindDc: trapFind, AreaSize: 1, Trigger: rulesv1.TrapTrigger_TRAP_TRIGGER_ENTER,
		Effect: &rulesv1.TrapEffect{Save: &rulesv1.TrapSaveEffect{
			Ability: rulesv1.Ability_ABILITY_DEXTERITY, Dc: trapSave, AppliesTo: rulesv1.TrapSaveApplies_TRAP_SAVE_APPLIES_CAUGHT,
			OnFail: &rulesv1.TrapOnFail{Damage: []*rulesv1.TrapDamage{{Dice: "2d6", DamageTypeKey: "damage-type:piercing"}}},
			OnPass: rulesv1.TrapPassOutcome_TRAP_PASS_OUTCOME_HALF,
		}},
	}
}

// buildStreamTargets makes the things the stream test changes (stream_test.go) while the players
// listen. They are made here so that the reads find them before and after the change.
func (w *world) buildStreamTargets() {
	ctx := w.t.Context()
	m := w.master
	// A trap in the south room, which the master will reveal to Caio's character alone.
	tx, ty := at(11, 13)
	w.pts["trap-caio"] = w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, w.secrets.marker("trap-caio-name"), w.secrets.marker("trap-caio-description"), 11, 13, func(r *mapsv1.CreateMapPointRequest) {
		r.Trap = w.trapSpec()
	})
	_, _ = tx, ty
	w.secrets.id("trap", w.pts["trap-caio"].GetId())
	// A trap that Ana's character will notice by walking by (a notice DC of 1 is always passed).
	w.pts["trap-noticed"] = w.point(w.fogMap, mapsv1.MapPointKind_MAP_POINT_KIND_TRAP, w.secrets.marker("trap-noticed-name"), w.secrets.marker("trap-noticed-description"), 4, 10, func(r *mapsv1.CreateMapPointRequest) {
		r.Trap = &mapsv1.TrapSpec{NoticeDc: 1, FindDc: trapFind, AreaSize: 1, Trigger: rulesv1.TrapTrigger_TRAP_TRIGGER_ENTER, Effect: &rulesv1.TrapEffect{}}
	})
	w.secrets.id("trap", w.pts["trap-noticed"].GetId())
	// An NPC the master will put on the stage, and a bandit the combat will get.
	w.stage2, _ = w.npc("stage2", &charactersv1.BasicSheet{HitPointsMax: 9, ArmorClass: 11, SpeedFt: 30})
	w.secrets.id("stage-npc-2", w.stage2.GetId())
	// A second map the master will reveal and make current for a moment.
	img := w.image("map2-image", 60, 40)
	w.map2 = must(m.maps.CreateMap(ctx, rq(&mapsv1.CreateMapRequest{CampaignId: w.campaign, Name: w.secrets.marker("map2-name"), ImageId: img}))).GetMap().GetId()
	w.secrets.id("map2", w.map2)
	w.map2Image = img
	w.secrets.id("gallery-image", img)
	// A milestone the master will mark reached, and a creature the master will give Caio's character.
	ms := must(m.xp.AddMilestone(ctx, rq(&progressionv1.AddMilestoneRequest{CampaignId: w.campaign, Text: w.secrets.marker("milestone-stream")}))).GetMilestone()
	w.milestoneStream = ms.GetId()
	w.secrets.id("milestone", ms.GetId())
}

// buildItems gives each player an item the master has not identified. What it looks like is
// the owner's to read; what it is (its name, key and numbers) is the master's, and so is the
// id of the line for everyone else.
func (w *world) buildItems() {
	ctx := w.t.Context()
	for _, g := range []struct {
		owner *person
		hero  *charactersv1.Character
		key   string
		name  string
	}{
		{w.ana, w.pens, "item:wand-of-fireballs", "Varinha de bolas de fogo"},
		{w.caio, w.toren, "item:ring-of-protection", "Anel de proteção"},
	} {
		res := must(w.master.inventory.GiveItems(ctx, rq(&charactersv1.GiveItemsRequest{
			CampaignId: w.campaign, CharacterId: g.hero.GetId(), IdempotencyKey: newKey(),
			Grants: []*charactersv1.ItemGrant{{CatalogKey: g.key, Unidentified: true, Look: w.secrets.marker("item-look-"+g.owner.name, g.owner)}},
		})))
		for _, e := range res.GetInventory().GetItems() {
			if e.GetCatalogKey() == g.key {
				w.secrets.id("item-"+g.owner.name, e.GetId(), g.owner)
			}
		}
		w.secrets.add(&canary{needle: g.name, kind: "item-identity-" + g.owner.name})
		w.secrets.add(&canary{needle: g.key, kind: "item-identity-" + g.owner.name})
	}
}
