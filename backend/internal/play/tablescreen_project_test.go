package play

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The projector of the table screen. Each test reads the same thing as the master, as the
// positive control, and as the TV, which must not carry what only the master reads. These tests need the
// database (MEURPG_TEST_DATABASE_URL).

// text is a message as text, to search for a word in everything it carries.
func text(m proto.Message) string { return prototext.Format(m) }

func mustContain(t *testing.T, what, got string, words ...string) {
	t.Helper()
	for _, w := range words {
		if !strings.Contains(got, w) {
			t.Errorf("%s does not carry %q", what, w)
		}
	}
}

// mustHaveNoNumber fails when an integer field of the message, however deep, holds n. A canary that is a number is
// searched this way and never as digits in the text, where a UUID or a timestamp holds them by chance.
func mustHaveNoNumber(t *testing.T, what string, m proto.Message, n int64) {
	t.Helper()
	var walk func(protoreflect.Message)
	walk = func(m protoreflect.Message) {
		m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			vals := []protoreflect.Value{v}
			if fd.IsList() {
				vals = vals[:0]
				for i := range v.List().Len() {
					vals = append(vals, v.List().Get(i))
				}
			}
			for _, e := range vals {
				switch fd.Kind() {
				case protoreflect.Int32Kind, protoreflect.Int64Kind, protoreflect.Sint32Kind, protoreflect.Sint64Kind, protoreflect.Sfixed32Kind, protoreflect.Sfixed64Kind:
					if e.Int() == n {
						t.Errorf("%s has the number %d in %s", what, n, fd.FullName())
					}
				case protoreflect.Uint32Kind, protoreflect.Uint64Kind, protoreflect.Fixed32Kind, protoreflect.Fixed64Kind:
					if strconv.FormatUint(e.Uint(), 10) == strconv.FormatInt(n, 10) {
						t.Errorf("%s has the number %d in %s", what, n, fd.FullName())
					}
				case protoreflect.MessageKind, protoreflect.GroupKind:
					if !fd.IsMap() {
						walk(e.Message())
					}
				}
			}
			return true
		})
	}
	walk(m.ProtoReflect())
}

func mustNotContain(t *testing.T, what, got string, words ...string) {
	t.Helper()
	for _, w := range words {
		if strings.Contains(got, w) {
			t.Errorf("%s carries %q, which the TV must not read", what, w)
		}
	}
}

// galleryImageNamed makes a gallery image with a name.
func (a *armed) galleryImageNamed(name string) string {
	a.h.t.Helper()
	id := newKey()
	if _, err := a.h.pool.Exec(a.h.t.Context(),
		`INSERT INTO gallery_images (id, campaign_id, name, content_type, width, height, byte_size, created_at) VALUES ($1, $2, $3, 'image/png', 40, 30, 100, now())`,
		id, a.campaignID, name); err != nil {
		a.h.t.Fatalf("insert a gallery image: %v", err)
	}
	return id
}

func (a *armed) showImage(t *testing.T, image string, aud *playv1.ImageAudience) {
	t.Helper()
	if _, err := a.master.play.SetShownImage(t.Context(), connect.NewRequest(&playv1.SetShownImageRequest{CampaignId: a.campaignID, ImageId: image, Audience: aud})); err != nil {
		t.Fatalf("SetShownImage() error = %v", err)
	}
}

func TestTableScreenImageIsTheOneShownToTheWholeTable(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	image := a.galleryImageNamed("Retrato do rei")
	a.showImage(t, image, &playv1.ImageAudience{Kind: playv1.ImageAudienceKind_IMAGE_AUDIENCE_KIND_WHOLE_TABLE})
	got := a.mustGetScreen(t)
	if got.GetStage().GetKind() != kImage {
		t.Fatalf("stage = %v, want the image", got.GetStage().GetKind())
	}
	img := got.GetStage().GetImage()
	if img.GetName() != "Retrato do rei" || img.GetUrl() != "/images/"+image || img.GetWidth() != 40 || img.GetHeight() != 30 {
		t.Errorf("image = %v, want the shown one", img)
	}
	// The master's own read is the positive control.
	live := a.master.liveSession(t, a.campaignID)
	if live.GetShownImage().GetName() != "Retrato do rei" {
		t.Fatalf("the master reads %v, want the image", live.GetShownImage())
	}
}

func TestTableScreenImageForChosenPlayersIsNotOnTheTV(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	image := a.galleryImageNamed("Retrato secreto do rei")
	a.showImage(t, image, &playv1.ImageAudience{Kind: playv1.ImageAudienceKind_IMAGE_AUDIENCE_KIND_CHOSEN_PLAYERS, UserIds: userIDs(a.ana)})
	if live := a.master.liveSession(t, a.campaignID); live.GetShownImage().GetName() != "Retrato secreto do rei" {
		t.Fatalf("the master reads %v, want the image", live.GetShownImage())
	}
	got := a.mustGetScreen(t)
	if got.GetStage().GetKind() == kImage || got.GetStage().GetImage() != nil {
		t.Errorf("stage = %v, want no image for chosen players", got.GetStage())
	}
	mustNotContain(t, "the table screen", text(got), "Retrato secreto", image)
	// A pin on the image falls to idle with the notice that says why.
	if _, err := a.setScreen(a.master, 0, false, kImage); err != nil {
		t.Fatalf("SetTableScreen() error = %v", err)
	}
	got = a.mustGetScreen(t)
	if got.GetStage().GetKind() != kIdle || got.GetNotice() != nPinEnded {
		t.Errorf("pinned: %v/%v, want idle/PIN_ENDED", got.GetStage().GetKind(), got.GetNotice())
	}
}

// sceneOnStage opens a scene with a goblin shown and the captain hidden on its stage.
func (a *armed) sceneOnStage(t *testing.T, audience ...*user) (point string) {
	t.Helper()
	point, _ = a.h.newScene(a.mapID, "A carroça tombada", false, 1)
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE map_points SET description = $2, hooks = $3 WHERE id = $1`, point, "Uma carroça virada na estrada.", "LEAKCANARY-hooks-1"); err != nil {
		t.Fatalf("describe the scene: %v", err)
	}
	if len(audience) > 0 {
		a.openFor(t, point, audience...)
	} else {
		a.openScene(t, point)
	}
	for _, id := range []string{a.goblin.GetId(), a.capitao.GetId()} {
		if err := a.putOnStage(t, id); err != nil {
			t.Fatalf("PutOnStage() error = %v", err)
		}
	}
	if _, err := a.master.play.SetStageNpcHidden(t.Context(), connect.NewRequest(&playv1.SetStageNpcHiddenRequest{CampaignId: a.campaignID, CharacterId: a.capitao.GetId(), Hidden: true})); err != nil {
		t.Fatalf("SetStageNpcHidden() error = %v", err)
	}
	return point
}

func TestTableScreenSceneCarriesTheNPCsTheTableSees(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	a.sceneOnStage(t)
	master := a.readScene(t, a.master)
	var masterNames []string
	for _, n := range master.GetStage() {
		masterNames = append(masterNames, n.GetName())
	}
	if len(masterNames) != 2 || master.GetHooks() == "" {
		t.Fatalf("the master reads stage %v and hooks %q, want both NPCs and the hooks", masterNames, master.GetHooks())
	}
	got := a.mustGetScreen(t)
	if got.GetStage().GetKind() != kScene {
		t.Fatalf("stage = %v, want the scene", got.GetStage().GetKind())
	}
	scene := got.GetStage().GetScene()
	if scene.GetName() != "A carroça tombada" || scene.GetDescription() != "Uma carroça virada na estrada." {
		t.Errorf("scene = %v, want the name and the description", scene)
	}
	if len(scene.GetStage()) != 1 || scene.GetStage()[0].GetName() != "Goblin" {
		t.Errorf("stage = %v, want only the goblin the table sees", scene.GetStage())
	}
	mustNotContain(t, "the table screen", text(got), "Capitão Goblin", "LEAKCANARY")
}

func TestTableScreenSceneForChosenPlayersIsNotOnTheTV(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	a.sceneOnStage(t, a.ana)
	if master := a.readScene(t, a.master); master.GetName() != "A carroça tombada" {
		t.Fatalf("the master reads %v, want the scene", master)
	}
	got := a.mustGetScreen(t)
	if got.GetStage().GetKind() == kScene || got.GetStage().GetScene() != nil || got.GetNotice() != nScene {
		t.Errorf("stage = %v with notice %v, want no scene and SCENE_NOT_FOR_ALL", got.GetStage(), got.GetNotice())
	}
	mustNotContain(t, "the table screen", text(got), "carroça", "Goblin", "LEAKCANARY")
}

// fight starts a combat of the party, the revealed goblin and an ogre of 4,321 hit points the master keeps
// hidden, who goes first and stands next to Toren.
func (a *armed) fightWithAnOgre(t *testing.T) (*playv1.Encounter, *playv1.Combatant) {
	t.Helper()
	ogre := a.master.npc(t, a.campaignID, "Ogro Oculto", 4321, 12)
	e := a.start(t, plan{
		npcs:     []*playv1.Participant{{CharacterId: ogre.GetId()}, {CharacterId: a.goblin.GetId()}},
		npcRolls: []int{25, 3},
		players:  map[string]int32{"Toren": 18, "Pensantus": 10, "Brisa": 1},
		reveal:   []string{"Goblin"},
		at:       map[string][2]int32{"Toren": {3, 3}, "Goblin": {4, 3}, "Pensantus": {10, 3}, "Brisa": {8, 8}, "Ogro Oculto": {3, 4}},
	})
	return e, byLabel(t, e, "Ogro Oculto")
}

func TestTableScreenCombatShowsOnlyWhatTheTableSees(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e, _ := a.fightWithAnOgre(t)
	// The master's own view is the positive control: the ogre, its numbers and its turn.
	master := text(a.get(t, a.master))
	mustContain(t, "the master's combat", master, "Ogro Oculto", "4321")

	got := a.mustGetScreen(t)
	if got.GetStage().GetKind() != kCombat {
		t.Fatalf("stage = %v, want the combat", got.GetStage().GetKind())
	}
	combat := got.GetStage().GetCombat()
	var names []string
	for _, c := range combat.GetOrder() {
		names = append(names, c.GetName())
	}
	if want := []string{"Toren", "Pensantus", "Brisa", "Goblin"}; fmt.Sprint(names) != fmt.Sprint(want) {
		t.Errorf("order = %v, want %v (no ogre)", names, want)
	}
	if !combat.GetMastersTurn() {
		t.Error("masters_turn = false, want true: the turn is the hidden ogre's")
	}
	for _, c := range combat.GetOrder() {
		wantState := playv1.TvState_TV_STATE_UNSPECIFIED // an up player character has no word
		if c.GetName() == "Goblin" {
			wantState = playv1.TvState_TV_STATE_UNHURT
		}
		if c.GetState() != wantState {
			t.Errorf("%s state = %v, want %v", c.GetName(), c.GetState(), wantState)
		}
		if c.GetOnTurn() {
			t.Errorf("%s is on turn, want nobody the table sees", c.GetName())
		}
		if c.GetIsPlayer() != (c.GetName() != "Goblin") || c.GetInitial() != c.GetName()[:1] {
			t.Errorf("%s: is_player %v, initial %q", c.GetName(), c.GetIsPlayer(), c.GetInitial())
		}
	}
	if combat.GetRound() != 1 {
		t.Errorf("round = %d, want 1", combat.GetRound())
	}
	mustNotContain(t, "the table screen", text(got), "Ogro", "hit_points", "armor_class")
	mustHaveNoNumber(t, "the table screen", got, 4321) // the ogre's hit points, by field: a UUID holds digits by chance

	// Hurt and down are words, never numbers.
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(4)); err != nil { // 7 -> 3
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	a.hurt(t, a.bri, 1) // fewer than half: a player character has no word while it is up
	a.hurt(t, a.pens, 0)
	state := map[string]playv1.TvState{}
	for _, c := range a.mustGetScreen(t).GetStage().GetCombat().GetOrder() {
		state[c.GetName()] = c.GetState()
	}
	if state["Goblin"] != playv1.TvState_TV_STATE_VERY_HURT || state["Brisa"] != playv1.TvState_TV_STATE_UNSPECIFIED || state["Pensantus"] != playv1.TvState_TV_STATE_FALLEN || state["Toren"] != playv1.TvState_TV_STATE_UNSPECIFIED {
		t.Errorf("states = %v, want Goblin very hurt, Pensantus fallen, Brisa and Toren with no word", state)
	}
	// A confirmed death is "defeated".
	deadToren(true)(t, a, e)
	for _, c := range a.mustGetScreen(t).GetStage().GetCombat().GetOrder() {
		if c.GetName() == "Toren" && c.GetState() != playv1.TvState_TV_STATE_DEFEATED {
			t.Errorf("dead Toren state = %v, want defeated", c.GetState())
		}
	}
}

func TestTableScreenCombatLogIsWhatEveryPlayerReads(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	e, _ := a.fightWithAnOgre(t)
	// The ogre (hidden) hits Toren; the master corrects the goblin's hit points; Toren attacks the goblin.
	a.mustAttack(t, a.master, e, "Ogro Oculto", sword, "Toren", d20(15))
	if _, err := a.adjustHP(t, e, "Goblin", damageHP(1)); err != nil {
		t.Fatalf("AdjustCombatantHitPoints() error = %v", err)
	}
	e = a.turnOf(t, "Toren")
	a.mustAttack(t, a.caio, e, "Toren", battleaxe, "Goblin", d20(16))

	masterLog := text(a.log(t, a.master, e))
	mustContain(t, "the master's log", masterLog, "Ogro Oculto", "HIT_POINTS_ADJUSTED")

	lines := a.mustGetScreen(t).GetStage().GetCombat().GetLog()
	if len(lines) == 0 || len(lines) > tvMaxLogLines {
		t.Fatalf("log = %q, want 1 to %d lines", lines, tvMaxLogLines)
	}
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "Toren atacou Goblin") || !strings.HasPrefix(lines[1], "O combate começou") {
		t.Errorf("log = %q, want Toren's attack and then the start of the combat", lines)
	}
	all := strings.Join(lines, "\n")
	mustNotContain(t, "the TV's log", all, "Ogro", "pontos de vida", "4321")
	if strings.Contains(all, "ajust") {
		t.Errorf("the master's correction is on the TV's log: %q", lines)
	}
}

func TestTableScreenRollsAreTheFreeRollsForTheTable(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	a.fightWithAnOgre(t)
	a.mustRollFree(a.master)
	a.mustRollFree(a.caio, func(r *playv1.RollFreeDiceRequest) { r.Modifier = 11 })
	a.mustRollFree(a.master, func(r *playv1.RollFreeDiceRequest) { r.Modifier = 19; r.Secret = true }) // "Só eu"
	a.mustRollFree(a.bia, func(r *playv1.RollFreeDiceRequest) { r.Modifier = 12 })
	a.mustRollFree(a.master, func(r *playv1.RollFreeDiceRequest) { r.Modifier = 13 })
	a.mustRollFree(a.master, func(r *playv1.RollFreeDiceRequest) { r.Modifier = 14 })

	masterRolls := text(a.freeRolls(a.master))
	mustContain(t, "the master's rolls", masterRolls, "2d6 + 19")

	rolls := a.mustGetScreen(t).GetStage().GetRolls()
	var got []string
	for _, r := range rolls {
		got = append(got, r.GetExpression())
		if len(r.GetFaces()) != 2 || r.GetWho() == "" {
			t.Errorf("roll = %v, want two faces and a roller", r)
		}
	}
	if want := []string{"2d6 + 14", "2d6 + 13", "2d6 + 12"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("rolls = %v, want the three newest for the table, newest first (no secret roll)", got)
	}
	// With neither a scene nor a combat nor a map there is no stage for the rolls.
	b := newArmed(t)
	b.mustRollFree(b.master)
	if r := b.mustGetScreen(t).GetStage().GetRolls(); len(r) != 0 {
		t.Errorf("the idle screen carries rolls %v", r)
	}
}

// RN-23: a line never names a table entry; the master may have hidden it.
func TestTableScreenLogNamesNoTableEntryTheMasterHid(t *testing.T) {
	t.Parallel()
	a := newTableCasters(t)
	e := a.castersFight(t, 2)
	a.mustCast(t, a.ana, e, "Pensantus", tableBolt, slotOfLevel(1), a.at(t, "Goblin 1"), castDisadvantage(15))
	if _, err := a.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: a.campaignID, Key: tableBolt})); err != nil {
		t.Fatalf("ArchiveTableEntry() error = %v", err)
	}
	mustContain(t, "the master's log", text(a.log(t, a.master, e)), "Raio de Nanquim")
	got := a.mustGetScreen(t)
	mustNotContain(t, "the table screen", text(got), "Nanquim", "raio-de-nanquim")
	lines := got.GetStage().GetCombat().GetLog()
	if len(lines) == 0 || lines[0] != "Pensantus conjurou uma magia" {
		t.Errorf("log = %q, want the cast without the entry's name", lines)
	}
}

func TestTvLogLineNamesNoTableContent(t *testing.T) {
	t.Parallel()
	cast := func(key, name string) *playv1.CombatLogEntry {
		return &playv1.CombatLogEntry{Kind: playv1.CombatLogKind_COMBAT_LOG_KIND_SPELL_CAST, ActorLabel: "Pensantus", Key: key, KeyNamePt: name}
	}
	for _, c := range []struct {
		entry *playv1.CombatLogEntry
		want  string
		ok    bool
	}{
		{cast("spell:fire-bolt", "Raio de Fogo"), "Pensantus conjurou Raio de Fogo", true},
		{cast("spell:raio-de-nanquim@mesa", "Raio de Nanquim"), "Pensantus conjurou uma magia", true},
		{&playv1.CombatLogEntry{Kind: playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION, ActorLabel: "Toren", Key: "feature:x@mesa:disengage", KeyNamePt: "Passo de Bruma"}, "Toren agiu", true},
		{&playv1.CombatLogEntry{Kind: playv1.CombatLogKind_COMBAT_LOG_KIND_ACTION, ActorLabel: "Garra-Torta", Key: "monster:garra-torta@mesa#claws", KeyNamePt: "Garras"}, "Garra-Torta agiu", true},
		{&playv1.CombatLogEntry{Kind: playv1.CombatLogKind_COMBAT_LOG_KIND_HIT_POINTS_ADJUSTED, TargetLabel: "Goblin"}, "", false},
		{&playv1.CombatLogEntry{Kind: playv1.CombatLogKind_COMBAT_LOG_KIND_LEGENDARY_SPENT, ActorLabel: "Dragão"}, "", false},
		{&playv1.CombatLogEntry{Kind: playv1.CombatLogKind_COMBAT_LOG_KIND_ATTACK, ActorLabel: "Toren"}, "", false},
	} {
		got, ok := tvLogLine(c.entry)
		if got != c.want || ok != c.ok {
			t.Errorf("tvLogLine(%v) = %q, %v; want %q, %v", c.entry, got, ok, c.want, c.ok)
		}
	}
}

// tvFields are the fields of the messages the projector fills, each with the reason it may reach the TV. A
// field that is not here fails TestTvProjectorFields: a new field is a decision made here, with its leak row.
var tvFields = map[string]string{
	"TvIdle":                "", // no fields
	"TvImage.name":          "the caption of the image shown to the whole table (MR-028)",
	"TvImage.url":           "where the TV fetches the image the whole table sees",
	"TvImage.width":         "the shown image's size",
	"TvImage.height":        "the shown image's size",
	"TvScene.name":          "the name of a scene open to the whole table, as every player reads it",
	"TvScene.description":   "the description of a scene open to the whole table, as every player reads it",
	"TvScene.stage":         "only the NPCs the table sees (the shown stage)",
	"TvStageNpc.name":       "the name of an NPC shown on the stage",
	"TvStageNpc.speaking":   "whether that NPC speaks",
	"TvCombat.round":        "the round, as every player reads it",
	"TvCombat.order":        "only combatants the table sees (RN-20)",
	"TvCombat.masters_turn": "whether the turn is a combatant the table does not see",
	"TvCombat.log":          "plain sentences from entries every player reads; no number, no table entry name (RN-23)",
	"TvCombatant.name":      "the label every player reads",
	"TvCombatant.initial":   "the first letter of the name",
	"TvCombatant.state":     "the RN-20 word, never hit points",
	"TvCombatant.on_turn":   "whose turn it is, among the combatants the table sees",
	"TvCombatant.is_player": "whether it is a player character",
	"TvRoll.who":            "the roller of a free roll for the table",
	"TvRoll.expression":     "the formula of a free roll for the table",
	"TvRoll.total":          "the total of a free roll for the table",
	"TvRoll.faces":          "the faces kept of a free roll for the table",
	"TvCombat.map":          "the combat's map, under the rules of TvMap",
	"TvMap.name":            "the map's name, as every player reads it; unset when hidden by the fog",
	"TvMap.hidden_by_fog":   "true when the fog is on and group vision is off (RN-10); then nothing else is set",
	"TvMap.map":             "the map, its points and its tokens from the player's GetMap code, for the TV's viewer",
	"TvMap.vision":          "the squares the group sees or remembers, from the player's GetMapVision code",
	"TvMap.revision":        "a hash of what the TV reads of the map; never the map's own revision",
}

// TestTvProjectorFields walks the messages the projector fills and fails for a field that has no reason in
// tvFields (a new field must be decided), and for a reason without a field (a removed field).
func TestTvProjectorFields(t *testing.T) {
	t.Parallel()
	messages := []protoreflect.MessageDescriptor{
		(&playv1.TvIdle{}).ProtoReflect().Descriptor(), (&playv1.TvImage{}).ProtoReflect().Descriptor(),
		(&playv1.TvScene{}).ProtoReflect().Descriptor(), (&playv1.TvStageNpc{}).ProtoReflect().Descriptor(),
		(&playv1.TvCombat{}).ProtoReflect().Descriptor(), (&playv1.TvCombatant{}).ProtoReflect().Descriptor(),
		(&playv1.TvRoll{}).ProtoReflect().Descriptor(), (&playv1.TvMap{}).ProtoReflect().Descriptor(),
	}
	seen := map[string]bool{"TvIdle": true}
	for _, md := range messages {
		fields := md.Fields()
		for i := range fields.Len() {
			f := fields.Get(i)
			key := string(md.Name()) + "." + string(f.Name())
			seen[key] = true
			if tvFields[key] == "" {
				t.Errorf("%s is a field the TV carries and has no reason in tvFields: decide it and add its leak row", key)
			}
		}
	}
	for key := range tvFields {
		if !seen[key] {
			t.Errorf("tvFields names %s, which is not a field of the messages", key)
		}
	}
}

// outsideOpenScene is asked by three callers: the stage's portraits for the image route, and the two readers
// of the group check. An empty user id is the TV: outside every scene that is not open to the whole table.
func TestOutsideOpenSceneCountsNobodyAsOutsideOnlyOfAWholeTableScene(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	ctx := t.Context()
	outside := func(userID string) bool {
		t.Helper()
		session, err := a.h.svc.queries.GetOpenGameSession(ctx, a.campaignID)
		if err != nil {
			t.Fatalf("GetOpenGameSession() error = %v", err)
		}
		got, err := outsideOpenScene(ctx, a.h.svc.queries, session, userID)
		if err != nil {
			t.Fatalf("outsideOpenScene() error = %v", err)
		}
		return got
	}
	ana, caio := a.ana.id, a.caio.id
	if outside("") || outside(ana) {
		t.Error("with no scene open, somebody is outside")
	}
	point, _ := a.h.newScene(a.mapID, "A clareira", false, 1)
	a.openFor(t, point, a.ana)
	if !outside("") || outside(ana) || !outside(caio) {
		t.Errorf("scene for Ana: outside(nobody) %v, Ana %v, Caio %v; want true, false, true", outside(""), outside(ana), outside(caio))
	}
	a.mustSetAudience(t, a.ana, a.caio)
	if !outside("") || outside(caio) {
		t.Error("scene for Ana and Caio: nobody in particular must be outside")
	}
	if _, err := a.setAudience(a.master, &playv1.SceneAudience{Everyone: true}); err != nil {
		t.Fatalf("SetSceneAudience(everyone) error = %v", err)
	}
	if outside("") || outside(ana) {
		t.Error("scene for everyone: nobody is outside")
	}
}

// The image route asks ImageOnStage with the viewer's id; the TV's empty id reads the stage of a scene open
// to the whole table only. ImageOnAnyStage is the question of whether the portrait is in use at all.
func TestImageOnStageForNobodyInParticular(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	ctx := t.Context()
	portrait := a.galleryImageNamed("Retrato do goblin")
	if _, err := a.h.pool.Exec(ctx, `UPDATE characters SET sheet = jsonb_set(sheet, '{basic,portrait_image_id}', to_jsonb($2::text)) WHERE id = $1`, a.goblin.GetId(), portrait); err != nil {
		t.Fatalf("set the portrait: %v", err)
	}
	point, _ := a.h.newScene(a.mapID, "A clareira", false, 1)
	a.openFor(t, point, a.ana)
	if err := a.putOnStage(t, a.goblin.GetId()); err != nil {
		t.Fatalf("PutOnStage() error = %v", err)
	}
	on := func(userID string) bool {
		t.Helper()
		got, err := a.h.svc.ImageOnStage(ctx, a.campaignID, userID, portrait)
		if err != nil {
			t.Fatalf("ImageOnStage() error = %v", err)
		}
		return got
	}
	if !on(a.ana.id) || on(a.caio.id) || on("") {
		t.Errorf("scene for Ana: Ana %v, Caio %v, nobody in particular %v; want true, false, false", on(a.ana.id), on(a.caio.id), on(""))
	}
	if inUse, err := a.h.svc.ImageOnAnyStage(ctx, a.campaignID, portrait); err != nil || !inUse {
		t.Errorf("ImageOnAnyStage() = %v, %v; want true: the portrait is in use", inUse, err)
	}
	if _, err := a.setAudience(a.master, &playv1.SceneAudience{Everyone: true}); err != nil {
		t.Fatalf("SetSceneAudience(everyone) error = %v", err)
	}
	if !on("") || !on(a.caio.id) {
		t.Error("scene for everyone: the portrait is on the stage for nobody in particular and for Caio")
	}
}

// An image left with the scene's players or with chosen players is not the TV's.
func TestTableScreenImageForTheScenesPlayersIsNotOnTheTV(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	point, _ := a.h.newScene(a.mapID, "A clareira", false, 1)
	a.openFor(t, point, a.ana)
	image := a.galleryImageNamed("Mapa da clareira")
	a.showImage(t, image, &playv1.ImageAudience{Kind: playv1.ImageAudienceKind_IMAGE_AUDIENCE_KIND_SCENE_PLAYERS})
	if live := a.master.liveSession(t, a.campaignID); live.GetShownImage().GetName() != "Mapa da clareira" {
		t.Fatalf("the master reads %v, want the image", live.GetShownImage())
	}
	if _, err := a.setScreen(a.master, 0, false, kImage); err != nil {
		t.Fatalf("SetTableScreen() error = %v", err)
	}
	got := a.mustGetScreen(t)
	if got.GetStage().GetKind() != kIdle || got.GetNotice() != nPinEnded || got.GetStage().GetImage() != nil {
		t.Errorf("stage = %v/%v, want idle/PIN_ENDED", got.GetStage().GetKind(), got.GetNotice())
	}
	mustNotContain(t, "the table screen", text(got), "Mapa da clareira", image)
}

// A stage whose item went away after the facts were read becomes the idle screen.
func TestProjectStageBodyFallsToIdleWhenTheItemIsGone(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	session, err := a.h.svc.queries.GetOpenGameSession(t.Context(), a.campaignID)
	if err != nil {
		t.Fatalf("GetOpenGameSession() error = %v", err)
	}
	for _, kind := range []playv1.TableScreenKind{kImage, kScene, kCombat} {
		stage := &playv1.TableScreenStage{Kind: kind}
		if err := a.h.svc.projectStageBody(t.Context(), session, stage); err != nil {
			t.Fatalf("projectStageBody(%v) error = %v", kind, err)
		}
		if stage.GetKind() != kIdle || stage.GetIdle() == nil || len(stage.GetRolls()) != 0 {
			t.Errorf("%v with nothing to show = %v, want the idle screen", kind, stage)
		}
	}
}

// With the fog on and "Visão do grupo" on, the TV sees the NPCs the group sees and no others; a line from when
// a player's character saw an NPC is not a line of the TV.
func TestTableScreenCombatOnAFogMapShowsOnlyNPCsTheGroupSees(t *testing.T) {
	t.Parallel()
	f := newFogCave(t)
	e := f.fight(t)
	f.stage(t)
	f.groupVision(t, true)
	f.mustMove(t, f.master, "Goblin 2", 7, 7)
	if _, err := f.attack(t, f.caio, e, "Toren", battleaxe, "Goblin 2", d20(15)); err != nil {
		t.Fatalf("RollAttack() error = %v", err)
	}
	f.mustMove(t, f.master, "Goblin 2", 8, 8)
	wantNPCs(t, "the master", f.get(t, f.master), allNPCs...)
	hidden := []string{"Goblin 1", "Goblin 3", "Ogro"}
	got := f.mustGetScreen(t)
	var npcs []string
	for _, c := range got.GetStage().GetCombat().GetOrder() {
		if !c.GetIsPlayer() {
			npcs = append(npcs, c.GetName())
		}
	}
	slices.Sort(npcs)
	if want := []string{"Capitão Goblin", "Escudeiro", "Goblin 2"}; !slices.Equal(npcs, want) {
		t.Errorf("NPCs on the TV = %v, want the ones the group sees %v", npcs, want)
	}
	f.noLeak(t, "the table screen", asJSON(t, got), hidden...)
	if lines := got.GetStage().GetCombat().GetLog(); len(lines) != 1 || lines[0] != "O combate começou" {
		t.Errorf("log = %q, want only the line that was not fogged", lines)
	}
	if !strings.Contains(asJSON(t, f.log(t, f.master, f.get(t, f.master))), "Goblin 2") {
		t.Error("the master's log does not carry the attack on Goblin 2")
	}
}

// RN-23: the player's read blanks the key of an NPC's stat block action, but the table screen decides on the key, so
// it asks for it whole. Without that a table monster's action ("monster:x@mesa#claws") would be named on the TV.
func TestTheTableScreenReadsTheWholeKeyOfAnNPCsAction(t *testing.T) {
	t.Parallel()
	const key = "monster:garra-torta@mesa#claws"
	npc := playdb.Combatant{Kind: kindNPC}
	if got := shownKey(combatViewer{}, npc, key); got != "" {
		t.Errorf("a player's key = %q, want it blank (RN-20)", got)
	}
	if got := shownKey(combatViewer{wholeKeys: true}, npc, key); got != key {
		t.Errorf("the table screen's key = %q, want it whole", got)
	}
}

// The log is read for the TV with whole keys: the way the TV's log reader is built is what keeps a table monster's
// action ("monster:x@mesa#claws") from being named.
func TestTheTVsLogViewerKeepsTheKeysWhole(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	v, err := a.h.svc.tvLogViewer(t.Context(), tvViewer(a.campaignID))
	if err != nil {
		t.Fatalf("tvLogViewer() error = %v", err)
	}
	const key = "monster:garra-torta@mesa#claws"
	if v.master {
		t.Fatal("the TV's log viewer is the master")
	}
	if got := shownKey(v, playdb.Combatant{Kind: kindNPC}, key); got != key || !rules.IsTableKeyAnywhere(got) {
		t.Errorf("the TV's key = %q, want %q whole", got, key)
	}
}
