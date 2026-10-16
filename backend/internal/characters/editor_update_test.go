package characters

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"uuid"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The characterization of UpdateCharacter and UpdateCharacterStory: every refusal with its
// code, message and detail, the order of the checks before the transaction and inside it,
// the portrait's copy, what package play is told about the gear, and the failures of the
// server.

const editorStale = "aborted: the character changed since you opened it; reload it and try again"

// editorUpdate calls UpdateCharacter as u.
func editorUpdate(u *user, req *charactersv1.UpdateCharacterRequest) (*charactersv1.Character, error) {
	res, err := u.api.UpdateCharacter(context.Background(), connect.NewRequest(proto.CloneOf(req)))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetCharacter(), nil
}

// editorSaveOf is the request that saves c as it is.
func editorSaveOf(c *charactersv1.Character) *charactersv1.UpdateCharacterRequest {
	return &charactersv1.UpdateCharacterRequest{
		CampaignId: c.GetCampaignId(), CharacterId: c.GetId(), Revision: c.GetRevision(), Name: c.GetName(), Sheet: proto.CloneOf(c.GetSheet()),
	}
}

// editorStandardFighter creates Dona's fighter with its scores made by the standard array.
func (et *editorTable) editorStandardFighter(t *testing.T) *charactersv1.Character {
	t.Helper()
	c, err := editorCreate(et.dona, &charactersv1.CreateCharacterRequest{
		CampaignId: et.campaign, Kind: editorPlayer, Name: "Bruna", Sheet: editorFighter(),
		AbilityMethod: charactersv1.AbilityMethod_ABILITY_METHOD_STANDARD_ARRAY,
	})
	if err != nil {
		t.Fatalf("CreateCharacter(standard array) error = %v", err)
	}
	return c
}

// TestUpdateCharacterEditorRefusalsBeforeTheTransaction: the request's own fields, in
// their order, and the callers who never reach the transaction.
func TestUpdateCharacterEditorRefusalsBeforeTheTransaction(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	pc := et.dona.createPensantus(t, et.campaign)
	req := editorSaveOf(pc)
	req.CharacterId, req.Revision, req.Name, req.Sheet = "x", 0, " ", nil
	steps := []struct {
		want string
		fix  func()
	}{
		{`not_found: character not found`, func() { req.CharacterId = pc.GetId() }},
		{`invalid_argument: revision must be at least 1 | InvalidField{field="revision"}`, func() { req.Revision = pc.GetRevision() }},
		{`invalid_argument: name must not be empty | InvalidField{field="name"}`, func() { req.Name = "Pensantus" }},
		{`invalid_argument: sheet is required: a full or a basic sheet | InvalidField{field="sheet"}`, func() { req.Sheet = pc.GetSheet() }},
		{`ok`, nil},
	}
	for i, step := range steps {
		_, err := editorUpdate(et.dona, req)
		wantEditorRefusal(t, "step "+strconv.Itoa(i), err, step.want)
		if step.fix != nil {
			step.fix()
		}
	}
	unknown := editorSaveOf(pc)
	unknown.CharacterId = uuid.New().String()
	for _, c := range []struct {
		name string
		as   *user
		req  *charactersv1.UpdateCharacterRequest
		want string
	}{
		{"nobody signed in", et.h.anonymous(), editorSaveOf(pc), `unauthenticated: sign in to continue`},
		{"a stranger", et.visitor, editorSaveOf(pc), `not_found: campaign not found`},
		{"another player", et.caio, editorSaveOf(pc), `not_found: character not found`},
		{"a pending member", et.pia, editorSaveOf(pc), `not_found: character not found`},
		{"a character that does not exist", et.master, unknown, `not_found: character not found`},
	} {
		_, err := editorUpdate(c.as, c.req)
		wantEditorRefusal(t, c.name, err, c.want)
	}
	// The pre-read of a portrait finds no character: the transaction answers.
	et.h.svc.SetGallery(&editorGallery{})
	goblin := et.master.create(t, et.campaign, editorMinion, "Goblin", basicSheet())
	missing := editorSaveOf(goblin)
	missing.CharacterId, missing.Sheet = uuid.New().String(), withPortrait(goblin.GetSheet(), uuid.New().String())
	_, err := editorUpdate(et.master, missing)
	wantEditorRefusal(t, "a portrait for a character that does not exist", err, `not_found: character not found`)
	// A player cannot save an NPC: it is not found to them.
	_, err = editorUpdate(et.dona, editorSaveOf(goblin))
	wantEditorRefusal(t, "a player saves an NPC", err, `not_found: character not found`)
}

// TestUpdateCharacterEditorRefusalsInsideTheTransaction: a save with every fault a player
// can make is refused for the first check the transaction makes; fixing that one shows the
// next. Then the locks of RN-01.
func TestUpdateCharacterEditorRefusalsInsideTheTransaction(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	archived := et.master.addEntry(t, et.campaign, testBackground("Guarda de farol"))
	if _, err := et.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: et.campaign, Key: archived.GetKey()})); err != nil {
		t.Fatalf("ArchiveTableEntry() error = %v", err)
	}
	pc := et.editorStandardFighter(t)
	req := editorSaveOf(pc)
	req.Revision = pc.GetRevision() + 1
	req.Sheet = basicSheet()
	full := editorClone(pc.GetSheet(), func(f *charactersv1.FullSheet) {
		f.PortraitImageId = uuid.New().String()
		f.Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: archived.GetKey()}
	}).GetFull()
	steps := []struct {
		want string
		fix  func()
	}{
		{`invalid_argument: sheet must be a full sheet for this kind of character | InvalidField{field="sheet"}`, func() {
			req.Sheet = &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: full}}
		}},
		{`invalid_argument: sheet.full.portrait_image_id must be empty for a player's character | InvalidField{field="sheet.full.portrait_image_id"}`, func() { full.PortraitImageId = "" }},
		{editorStale, func() { req.Revision = pc.GetRevision() }},
		{`failed_precondition: the master archived a table entry this sheet picks: it is not a new choice any more | CharacterBlocked{reason=CHARACTER_BLOCKED_REASON_ARCHIVED_CONTENT character_id="` + pc.GetId() + `" content_key="` + archived.GetKey() + `"}`, func() {
			full.Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"}
			full.FeatureChoiceKeys = nil
		}},
		{`failed_precondition: the sheet has choices still to make: complete them first | ChoiceRefusal{reason=CHOICE_REFUSAL_REASON_CHOICES_MISSING issues=[ChoiceIssue{field="sheet.full.feature_choice_keys" choice_key="feature:fighter-fighting-style" required=1 label_pt="Estilo de Luta (Guerreiro, nível 1)"}]}`, func() {
			full.FeatureChoiceKeys = []string{"feature:fighter-fighting-style-defense"}
			full.Classes = append(full.Classes, &charactersv1.ClassLevel{ClassKey: "class:wizard", Level: 1})
		}},
		{`failed_precondition: the level up does not allow this choice: sheet.full.classes[1].class_key | LevelUpRefusal{field="sheet.full.classes[1].class_key" reason=LEVEL_UP_REFUSAL_REASON_MULTICLASS_PREREQUISITE class_key="class:wizard" ability=ABILITY_INTELLIGENCE minimum=13 have=11}`, func() {
			full.Classes = full.Classes[:1]
			full.FeatKeys = []string{"feat:grappler"}
		}},
		{`permission_denied: only the campaign's master adds or removes the feats of a sheet: a player's feats come from the guided level-up`, func() {
			full.FeatKeys = nil
			full.ExtraAbilityBonuses = &rulesv1.AbilityScores{Strength: 3}
		}},
		{`failed_precondition: the ability scores are refused: ABILITY_SCORES_REFUSAL_REASON_EXTRA_BONUSES | AbilityScoresRefusal{reason=ABILITY_SCORES_REFUSAL_REASON_EXTRA_BONUSES method=ABILITY_METHOD_STANDARD_ARRAY}`, func() {
			full.ExtraAbilityBonuses = nil
			full.BaseScores.Charisma = 9
		}},
		{`failed_precondition: the ability scores are refused: ABILITY_SCORES_REFUSAL_REASON_NOT_STANDARD_ARRAY | AbilityScoresRefusal{reason=ABILITY_SCORES_REFUSAL_REASON_NOT_STANDARD_ARRAY method=ABILITY_METHOD_STANDARD_ARRAY}`, func() {
			full.BaseScores.Charisma = 8
			full.AbilityOrigin = &charactersv1.AbilityOrigin{Method: charactersv1.AbilityMethod_ABILITY_METHOD_TYPED}
		}},
		{`ok`, nil},
	}
	for i, step := range steps {
		_, err := editorUpdate(et.dona, req)
		wantEditorRefusal(t, "step "+strconv.Itoa(i), err, step.want)
		if step.fix != nil {
			step.fix()
		}
	}
	saved := et.dona.get(t, et.campaign, pc.GetId())
	if o := editorMessage(saved.GetSheet().GetFull().GetAbilityOrigin().ProtoReflect()); o != "AbilityOrigin{method=ABILITY_METHOD_STANDARD_ARRAY}" {
		t.Errorf("the origin after the save = %s, want the server's, not the client's", o)
	}
	// The master's edit of the same scores is free.
	byMaster := editorSaveOf(saved)
	byMaster.Sheet.GetFull().BaseScores.Charisma = 9
	if saved, err := editorUpdate(et.master, byMaster); err != nil {
		t.Errorf("the master's edit of the scores: %v", err)
	} else {
		byMaster = editorSaveOf(saved)
	}

	// RN-01: once the sheets lock, the player's save is refused before its kind and its
	// revision are checked; the master still saves.
	et.h.lockSheets(et.campaign)
	locked := editorSaveOf(saved)
	locked.Revision, locked.Sheet = 99, basicSheet()
	_, err := editorUpdate(et.dona, locked)
	wantEditorRefusal(t, "a locked sheet", err, `failed_precondition: the sheet is locked: a game session has started, so only the master edits it | CharacterBlocked{reason=CHARACTER_BLOCKED_REASON_SHEET_LOCKED character_id="`+pc.GetId()+`"}`)
	byMaster.Name = "Bruna, a Ousada"
	if _, err := editorUpdate(et.master, byMaster); err != nil {
		t.Errorf("the master's save of a locked sheet: %v", err)
	}
	dead := et.master.markDead(t, et.dona.get(t, et.campaign, pc.GetId()))
	_, err = editorUpdate(et.dona, editorSaveOf(dead))
	wantEditorRefusal(t, "a dead character", err, `failed_precondition: the character is dead, so only the master edits its sheet | CharacterBlocked{reason=CHARACTER_BLOCKED_REASON_CHARACTER_DEAD character_id="`+pc.GetId()+`"}`)

	// A pending member edits their pending character.
	pending, err := editorCreate(et.pia, &charactersv1.CreateCharacterRequest{CampaignId: et.campaign, Kind: editorPlayer, Name: "Pia", Sheet: editorFighter()})
	if err != nil {
		t.Fatalf("the pending member's create: %v", err)
	}
	edit := editorSaveOf(pending)
	edit.Name = "Pia, a Paciente"
	if _, err := editorUpdate(et.pia, edit); err != nil {
		t.Errorf("the pending member's save: %v", err)
	}
}

// TestUpdateCharacterEditorPortraits: the portrait's copy is prepared before the
// transaction, its gallery row is written in it, and its files go when the save does
// not happen or the row was there already.
func TestUpdateCharacterEditorPortraits(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	image, fog, copyID := uuid.New().String(), uuid.New().String(), uuid.New().String()
	gal := &editorGallery{images: map[string][]string{et.campaign: {image, fog}}, fog: fog, copyID: copyID}
	et.h.svc.SetGallery(gal)
	goblin := et.master.create(t, et.campaign, editorMinion, "Goblin", basicSheet())
	counts := func() string {
		return "prepared " + strconv.Itoa(int(gal.prepared.Load())) + ", inserted " + strconv.Itoa(int(gal.inserted.Load())) + ", discarded " + strconv.Itoa(int(gal.discarded.Load()))
	}

	// An image of no gallery comes before a stale revision.
	bad := editorSaveOf(goblin)
	bad.Revision, bad.Sheet = 99, withPortrait(goblin.GetSheet(), uuid.New().String())
	_, err := editorUpdate(et.master, bad)
	wantEditorRefusal(t, "an image of no gallery", err, `invalid_argument: sheet.basic.portrait_image_id must be an image of the campaign's gallery | InvalidField{field="sheet.basic.portrait_image_id"}`)
	// A gallery that fails stops the save before its transaction.
	gal.failCheck.Store(true)
	_, err = editorUpdate(et.master, editorSaveOf(goblin))
	if err != nil {
		t.Errorf("a save without a portrait asked the gallery: %v", err)
	}
	goblin = et.master.get(t, et.campaign, goblin.GetId())
	with := editorSaveOf(goblin)
	with.Sheet = withPortrait(goblin.GetSheet(), image)
	_, err = editorUpdate(et.master, with)
	wantEditorRefusal(t, "a gallery that fails", err, `internal: internal error`)
	et.editorLogged(t, `"msg":"characters: cannot check a portrait"`, `the gallery is down`)
	gal.failCheck.Store(false)

	// A copy whose row fails in the transaction: the save fails, the files go.
	gal.failInsert.Store(true)
	with.Sheet = withPortrait(goblin.GetSheet(), fog)
	_, err = editorUpdate(et.master, with)
	wantEditorRefusal(t, "a copy that fails", err, `internal: internal error`)
	et.editorLogged(t, `"msg":"characters: cannot update a character"`, `the gallery is down`)
	if got := counts(); got != "prepared 1, inserted 0, discarded 1" {
		t.Errorf("after a copy that fails: %s", got)
	}
	gal.failInsert.Store(false)

	// A stale save with a copy: the files go.
	stale := proto.CloneOf(with)
	stale.Revision = 99
	_, err = editorUpdate(et.master, stale)
	wantEditorRefusal(t, "a stale save with a copy", err, editorStale)
	if got := counts(); got != "prepared 2, inserted 0, discarded 2" {
		t.Errorf("after a stale save: %s", got)
	}
	// A save that happens keeps the copy, which is the portrait.
	saved, err := editorUpdate(et.master, with)
	if err != nil || portraitOf(saved.GetSheet()) != copyID {
		t.Errorf("a save with a copy: %v, %v; want the copy's ID as the portrait", portraitOf(saved.GetSheet()), err)
	}
	if got := counts(); got != "prepared 3, inserted 1, discarded 2" {
		t.Errorf("after a save: %s", got)
	}
	// A copy whose row was there already: the save happens and the new files go.
	gal.reused.Store(true)
	again := editorSaveOf(saved)
	again.Sheet = withPortrait(saved.GetSheet(), fog)
	if _, err := editorUpdate(et.master, again); err != nil {
		t.Errorf("a save with a reused copy: %v", err)
	}
	if got := counts(); got != "prepared 4, inserted 2, discarded 3" {
		t.Errorf("after a reused copy: %s", got)
	}
	// Nobody who may not save the sheet makes the server copy anything.
	pc := et.dona.createPensantus(t, et.campaign)
	et.h.lockSheets(et.campaign)
	for _, c := range []struct {
		name string
		as   *user
		req  *charactersv1.UpdateCharacterRequest
		want string
	}{
		{"a player on an NPC", et.dona, again, `not_found: character not found`},
		{"a locked player's sheet", et.dona, func() *charactersv1.UpdateCharacterRequest {
			r := editorSaveOf(pc)
			r.Sheet = withPortrait(pc.GetSheet(), fog)
			return r
		}(), `failed_precondition: the sheet is locked: a game session has started, so only the master edits it | CharacterBlocked{reason=CHARACTER_BLOCKED_REASON_SHEET_LOCKED character_id="` + pc.GetId() + `"}`},
		{"a basic sheet for a player's character", et.master, func() *charactersv1.UpdateCharacterRequest {
			r := editorSaveOf(pc)
			r.Sheet = withPortrait(basicSheet(), fog)
			return r
		}(), `invalid_argument: sheet must be a full sheet for this kind of character | InvalidField{field="sheet"}`},
	} {
		_, err := editorUpdate(c.as, c.req)
		wantEditorRefusal(t, c.name, err, c.want)
	}
	if got := counts(); got != "prepared 4, inserted 2, discarded 3" {
		t.Errorf("after the callers who may not save: %s, want nothing more prepared", got)
	}
}

// TestUpdateCharacterEditorTellsPlayAboutTheGear: a save that dons armor tells play
// ArmorWorn; one that changes the shield, or takes the armor off, GearChanged; the call
// play returns runs after the commit. Nothing else tells play anything.
func TestUpdateCharacterEditorTellsPlayAboutTheGear(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	spy := &gearHostSpy{}
	et.h.svc.SetReviewHost(spy)
	npc := et.master.create(t, et.campaign, editorEnemy, "Bandido", enemySheet())
	steps := []struct {
		name string
		edit func(f *charactersv1.FullSheet)
		want string
	}{
		{"the name only", func(*charactersv1.FullSheet) {}, ""},
		{"the shield off", func(f *charactersv1.FullSheet) { f.Shield = false }, "GearChanged, after GearChanged"},
		{"the armor off", func(f *charactersv1.FullSheet) { f.ArmorKey = "" }, "GearChanged, after GearChanged"},
		{"armor on", func(f *charactersv1.FullSheet) { f.ArmorKey = "equipment:leather-armor" }, "ArmorWorn, after ArmorWorn"},
		{"other armor and the shield", func(f *charactersv1.FullSheet) { f.ArmorKey, f.Shield = "equipment:chain-mail", true }, "ArmorWorn, after ArmorWorn"},
		{"a weapon", func(f *charactersv1.FullSheet) { f.WeaponKeys = []string{"equipment:dagger"} }, ""},
	}
	for i, step := range steps {
		req := editorSaveOf(npc)
		req.Name = "Bandido " + strconv.Itoa(i)
		step.edit(req.Sheet.GetFull())
		saved, err := editorUpdate(et.master, req)
		if err != nil {
			t.Fatalf("%s: UpdateCharacter() error = %v", step.name, err)
		}
		if got := spy.take(); got != step.want {
			t.Errorf("%s: play was told %q, want %q", step.name, got, step.want)
		}
		npc = saved
	}
	// A refused save tells play nothing.
	stale := editorSaveOf(npc)
	stale.Revision++
	stale.Sheet.GetFull().ArmorKey = ""
	_, err := editorUpdate(et.master, stale)
	wantEditorRefusal(t, "a stale save", err, editorStale)
	if got := spy.take(); got != "" {
		t.Errorf("a refused save told play %q", got)
	}
}

// TestUpdateCharacterEditorChecksAMovedContentAgain: a content that moved between the
// handler's read and its transaction checks the sheet again: a sheet the new content
// refuses is refused, one it takes is saved with its revision.
func TestUpdateCharacterEditorChecksAMovedContentAgain(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	bg := et.master.addEntry(t, et.campaign, testBackground("Guarda de farol"))
	pc := et.dona.create(t, et.campaign, editorPlayer, "Bruna", editorClone(editorFighter(), func(f *charactersv1.FullSheet) {
		f.Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: bg.GetKey()}
	}))
	et.src.mu.Lock()
	et.src.hook = func() { et.master.addEntry(t, et.campaign, testBackground("Outro fundo")) }
	et.src.mu.Unlock()
	saved, err := editorUpdate(et.dona, editorSaveOf(pc))
	if err != nil || saved.GetSheet().GetFull().GetContentRevision() != 2 {
		t.Fatalf("a save across a content write = revision %d, %v; want 2", saved.GetSheet().GetFull().GetContentRevision(), err)
	}
	// Inside the transaction, the content of a campaign without the background (and at
	// another revision): the sheet checked again is refused.
	other := et.h.newCampaign(et.master, "Outra")
	et.master.addEntry(t, other, testBackground("Um"))
	et.master.addEntry(t, other, testBackground("Dois"))
	et.master.addEntry(t, other, testBackground("Três"))
	et.src.mu.Lock()
	et.src.swapInside = map[string]string{et.campaign: other}
	et.src.mu.Unlock()
	_, err = editorUpdate(et.dona, editorSaveOf(saved))
	wantEditorRefusal(t, "a sheet the moved content refuses", err, `invalid_argument: sheet.full.background_key is not a known background | InvalidField{field="sheet.full.background_key"}`)
}

// TestUpdateCharacterEditorWhenTheServerFails: a content that cannot be read and a stored
// sheet that cannot be read are the server's failures, logged with what failed.
func TestUpdateCharacterEditorWhenTheServerFails(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	pc := et.dona.createPensantus(t, et.campaign)
	et.src.failOutside.Store(true)
	_, err := editorUpdate(et.dona, editorSaveOf(pc))
	wantEditorRefusal(t, "the first read fails", err, `internal: internal error`)
	et.editorLogged(t, `"msg":"characters: cannot read rules content"`, `the content source is down`)
	et.src.failOutside.Store(false)
	et.src.failInside.Store(true)
	_, err = editorUpdate(et.dona, editorSaveOf(pc))
	wantEditorRefusal(t, "the read in the transaction fails", err, `internal: internal error`)
	et.editorLogged(t, `"msg":"characters: cannot update a character"`, `read rules content: the content source is down`)
	et.src.failInside.Store(false)

	et.editorCorrupt(t, pc.GetId(), "sheet")
	_, err = editorUpdate(et.dona, editorSaveOf(pc))
	wantEditorRefusal(t, "a stored sheet that cannot be read", err, `internal: this character cannot be read right now`)
	et.editorLogged(t, `"msg":"characters: cannot update a character"`, `the sheet of character `+pc.GetId())
}

// editorStory calls UpdateCharacterStory as u.
func editorStory(u *user, req *charactersv1.UpdateCharacterStoryRequest) (*charactersv1.Character, error) {
	res, err := u.api.UpdateCharacterStory(context.Background(), connect.NewRequest(proto.CloneOf(req)))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetCharacter(), nil
}

// TestUpdateCharacterStoryEditorRefusals: the request's fields in their order, who may
// write a story when, and the story's lock before the revision.
func TestUpdateCharacterStoryEditorRefusals(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	pc := et.dona.createPensantus(t, et.campaign)
	req := &charactersv1.UpdateCharacterStoryRequest{
		CampaignId: et.campaign, CharacterId: "x", Revision: 0,
		Story: &charactersv1.CharacterStory{Personality: &charactersv1.Personality{Bonds: strings.Repeat("x", maxPersonalityLength+1)}},
	}
	steps := []struct {
		want string
		fix  func()
	}{
		{`not_found: character not found`, func() { req.CharacterId = pc.GetId() }},
		{`invalid_argument: revision must be at least 1 | InvalidField{field="revision"}`, func() { req.Revision = pc.GetRevision() + 1 }},
		{`invalid_argument: story.personality.bonds must be at most 1000 characters, got 1001 | InvalidField{field="story.personality.bonds"}`, func() {
			req.Story = &charactersv1.CharacterStory{Backstory: "Nasceu em Mirathel."}
		}},
		{editorStale, func() { req.Revision = pc.GetRevision() }},
		{`ok`, nil},
	}
	for i, step := range steps {
		_, err := editorStory(et.dona, req)
		wantEditorRefusal(t, "step "+strconv.Itoa(i), err, step.want)
		if step.fix != nil {
			step.fix()
		}
	}
	current := et.dona.get(t, et.campaign, pc.GetId())
	for _, c := range []struct {
		name string
		as   *user
		want string
	}{
		{"nobody signed in", et.h.anonymous(), `unauthenticated: sign in to continue`},
		{"a stranger", et.visitor, `not_found: campaign not found`},
		{"another player", et.caio, `not_found: character not found`},
		{"a pending member", et.pia, `not_found: character not found`},
	} {
		r := proto.CloneOf(req)
		r.Revision = current.GetRevision()
		_, err := editorStory(c.as, r)
		wantEditorRefusal(t, c.name, err, c.want)
	}
	// RN-01: locked, the player's story is refused before its revision; the master
	// writes it, and lets the player write it again.
	et.h.lockSheets(et.campaign)
	locked := proto.CloneOf(req)
	locked.Revision = 99
	_, err := editorStory(et.dona, locked)
	wantEditorRefusal(t, "a locked story", err, `failed_precondition: the story is locked: ask the master to allow editing it | CharacterBlocked{reason=CHARACTER_BLOCKED_REASON_STORY_LOCKED character_id="`+pc.GetId()+`"}`)
	current = et.dona.get(t, et.campaign, pc.GetId())
	byMaster := proto.CloneOf(req)
	byMaster.Revision = current.GetRevision()
	written, err := editorStory(et.master, byMaster)
	if err != nil {
		t.Fatalf("the master's story: %v", err)
	}
	allowed := et.master.setStoryEditing(t, written, true)
	byPlayer := proto.CloneOf(req)
	byPlayer.Revision = allowed.GetRevision()
	byPlayer.Story.Allies = "Os magos de Mirathel."
	got, err := editorStory(et.dona, byPlayer)
	if err != nil || got.GetStory().GetAllies() != "Os magos de Mirathel." {
		t.Errorf("the player's story with the master's leave = %v, %v", got.GetStory(), err)
	}
	// An NPC's story is the master's.
	npc := et.master.create(t, et.campaign, editorMinion, "Goblin", basicSheet())
	if _, err := editorStory(et.master, &charactersv1.UpdateCharacterStoryRequest{CampaignId: et.campaign, CharacterId: npc.GetId(), Revision: npc.GetRevision()}); err != nil {
		t.Errorf("the master's story of an NPC: %v", err)
	}
	// A content that cannot be read is the server's failure, and nothing is written.
	et.src.failOutside.Store(true)
	byPlayer.Revision = got.GetRevision()
	_, err = editorStory(et.dona, byPlayer)
	wantEditorRefusal(t, "the content fails", err, `internal: internal error`)
	et.editorLogged(t, `"msg":"characters: cannot read rules content"`)
	et.src.failOutside.Store(false)
	if after := et.dona.get(t, et.campaign, pc.GetId()); after.GetRevision() != got.GetRevision() {
		t.Errorf("the failed story moved the revision from %d to %d", got.GetRevision(), after.GetRevision())
	}
}
