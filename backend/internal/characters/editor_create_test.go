package characters

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"uuid"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The characterization of CreateCharacter: every refusal with its code, message and
// detail, the order of the checks, who creates what, the columns a create writes and the
// log line it leaves.

const (
	editorPlayer = charactersv1.CharacterKind_CHARACTER_KIND_PLAYER
	editorEnemy  = charactersv1.CharacterKind_CHARACTER_KIND_ENEMY
	editorMinion = charactersv1.CharacterKind_CHARACTER_KIND_MINION
)

// editorCreate calls CreateCharacter as u.
func editorCreate(u *user, req *charactersv1.CreateCharacterRequest) (*charactersv1.Character, error) {
	res, err := u.api.CreateCharacter(context.Background(), connect.NewRequest(proto.CloneOf(req)))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetCharacter(), nil
}

// editorFighter is a human fighter of level 1 with the standard array, the Defense style
// picked and the acolyte's background: a player's sheet with nothing left open.
func editorFighter() *charactersv1.CharacterSheet {
	return choiceSheet("race:human", "", "class:fighter", "", 1, "feature:fighter-fighting-style-defense")
}

// TestCreateCharacterEditorRefusalsComeInOrder: a request with every fault the player can
// make is refused for the first check only; fixing that one shows the next, so the order
// of the checks before the transaction is frozen with the answer of each.
func TestCreateCharacterEditorRefusalsComeInOrder(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	et.h.svc.SetGallery(&editorGallery{})
	archived := et.master.addEntry(t, et.campaign, testBackground("Guarda de farol"))
	if _, err := et.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: et.campaign, Key: archived.GetKey()})); err != nil {
		t.Fatalf("ArchiveTableEntry() error = %v", err)
	}
	sheet := choiceSheet("race:human", "", "class:fighter", "", 1)
	full := sheet.GetFull()
	full.Equipment = []*charactersv1.Item{{Name: " "}}
	full.ChallengeRating = "1"
	full.Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: archived.GetKey()}
	full.FeatKeys = []string{"feat:grappler"}
	full.PortraitImageId = uuid.New().String()
	req := &charactersv1.CreateCharacterRequest{
		CampaignId: et.campaign, AbilityMethod: 99, Name: " ", Sheet: sheet,
		Story:          &charactersv1.CharacterStory{Backstory: strings.Repeat("x", maxBackstoryLength+1)},
		IdempotencyKey: "x",
	}
	steps := []struct {
		want string
		fix  func()
	}{
		{`invalid_argument: kind is required | InvalidField{field="kind"}`, func() { req.Kind = editorPlayer }},
		{`invalid_argument: ability_method is not a known method | InvalidField{field="ability_method"}`, func() { req.AbilityMethod = 0 }},
		{`invalid_argument: name must not be empty | InvalidField{field="name"}`, func() { req.Name = "Bruna" }},
		{`invalid_argument: sheet.full.equipment[0].name must not be empty | InvalidField{field="sheet.full.equipment[0].name"}`, func() { full.Equipment = nil }},
		{`invalid_argument: sheet.full.challenge_rating must be empty for a player's character | InvalidField{field="sheet.full.challenge_rating"}`, func() { full.ChallengeRating = "" }},
		{`failed_precondition: the master archived a table entry this sheet picks: it is not a new choice any more | CharacterBlocked{reason=CHARACTER_BLOCKED_REASON_ARCHIVED_CONTENT content_key="` + archived.GetKey() + `"}`, func() {
			full.Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"}
		}},
		{`failed_precondition: the sheet has choices still to make: complete them first | ChoiceRefusal{reason=CHOICE_REFUSAL_REASON_CHOICES_MISSING issues=[ChoiceIssue{field="sheet.full.feature_choice_keys" choice_key="feature:fighter-fighting-style" required=1 label_pt="Estilo de Luta (Guerreiro, nível 1)"}]}`, func() {
			full.FeatureChoiceKeys = []string{"feature:fighter-fighting-style-defense"}
		}},
		{`permission_denied: only the campaign's master adds or removes the feats of a sheet: a player's feats come from the guided level-up`, func() { full.FeatKeys = nil }},
		{`invalid_argument: sheet.full.portrait_image_id must be empty for a player's character | InvalidField{field="sheet.full.portrait_image_id"}`, func() { full.PortraitImageId = "" }},
		{`invalid_argument: story.backstory must be at most 10000 characters, got 10001 | InvalidField{field="story.backstory"}`, func() { req.Story = nil }},
		{`invalid_argument: idempotency_key must be a UUID | InvalidField{field="idempotency_key"}`, func() { req.IdempotencyKey = uuid.New().String() }},
		{`ok`, nil},
	}
	for i, step := range steps {
		_, err := editorCreate(et.dona, req)
		wantEditorRefusal(t, "step "+strconv.Itoa(i), err, step.want)
		if step.fix != nil {
			step.fix()
		}
	}
	if n := et.editorCount(t); n != 1 {
		t.Errorf("the campaign has %d characters after the refusals and one create, want 1", n)
	}
}

// TestCreateCharacterEditorWhoCreatesWhat: the master creates NPCs and the reserved
// characters for a player to claim; a player and a pending member create their own; the
// answers to everybody else, and to a kind the caller may not make, come before any check
// of the request's other fields.
func TestCreateCharacterEditorWhoCreatesWhat(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	badName := strings.Repeat("x", MaxNameLength+1)
	refusals := []struct {
		name      string
		as        *user
		kind      charactersv1.CharacterKind
		forPlayer bool
		want      string
	}{
		{"nobody signed in", et.h.anonymous(), editorPlayer, false, `unauthenticated: sign in to continue`},
		{"a stranger", et.visitor, editorPlayer, false, `not_found: campaign not found`},
		{"the master, a player character", et.master, editorPlayer, false, `permission_denied: only a player creates a player character; the master creates NPCs`},
		{"a player, an NPC", et.dona, editorEnemy, false, `permission_denied: only the campaign's master creates NPCs`},
		{"a pending member, an NPC", et.pia, editorMinion, false, `permission_denied: only the campaign's master creates NPCs`},
		{"a player, for a player", et.dona, editorPlayer, true, `permission_denied: only the campaign's master makes a character for a player`},
		{"the master, an NPC for a player", et.master, editorEnemy, true, `invalid_argument: for_player needs the kind PLAYER | InvalidField{field="for_player"}`},
		{"the master, a name too long", et.master, editorEnemy, false, `invalid_argument: name must be at most 80 characters, got 81 | InvalidField{field="name"}`},
	}
	for _, r := range refusals {
		// Every request also carries an unknown ability method and a name too long: the
		// kind's answer comes first.
		_, err := editorCreate(r.as, &charactersv1.CreateCharacterRequest{
			CampaignId: et.campaign, Kind: r.kind, ForPlayer: r.forPlayer, Name: badName, Sheet: enemySheet(),
		})
		wantEditorRefusal(t, r.name, err, r.want)
	}
	_, err := editorCreate(et.master, &charactersv1.CreateCharacterRequest{CampaignId: et.campaign, Kind: editorEnemy, Name: "Bandido", AbilityMethod: 99, Sheet: enemySheet()})
	wantEditorRefusal(t, "an NPC with an unknown ability method", err, `invalid_argument: ability_method is not a known method | InvalidField{field="ability_method"}`)

	creates := []struct {
		name      string
		as        *user
		kind      charactersv1.CharacterKind
		forPlayer bool
		sheet     *charactersv1.CharacterSheet
		row       string
	}{
		{"an enemy", et.master, editorEnemy, false, enemySheet(), "player=none master=Mestre status=active reserved=false"},
		{"a boss", et.master, charactersv1.CharacterKind_CHARACTER_KIND_BOSS, false, enemySheet(), "player=none master=Mestre status=active reserved=false"},
		{"a minion", et.master, editorMinion, false, basicSheet(), "player=none master=Mestre status=active reserved=false"},
		{"a story NPC", et.master, charactersv1.CharacterKind_CHARACTER_KIND_STORY, false, basicSheet(), "player=none master=Mestre status=active reserved=false"},
		{"a reserved player character", et.master, editorPlayer, true, pensantusSheet(), "player=none master=none status=active reserved=true"},
		{"a player's own", et.dona, editorPlayer, false, pensantusSheet(), "player=Dona master=none status=active reserved=false"},
		{"a pending member's own", et.pia, editorPlayer, false, pensantusSheet(), "player=Pia master=none status=pending reserved=false"},
	}
	for _, c := range creates {
		got, err := editorCreate(c.as, &charactersv1.CreateCharacterRequest{CampaignId: et.campaign, Kind: c.kind, ForPlayer: c.forPlayer, Name: "Teste", Sheet: c.sheet})
		if err != nil {
			t.Errorf("%s: CreateCharacter() error = %v", c.name, err)
			continue
		}
		if row := et.editorRow(t, got.GetId()); row != c.row {
			t.Errorf("%s: stored %s, want %s", c.name, row, c.row)
		}
		et.editorLogged(t, `"msg":"event"`, `"event":"character.created"`, `"character_id":"`+got.GetId()+`"`, `"kind":"`+strings.ToLower(strings.TrimPrefix(c.kind.String(), "CHARACTER_KIND_"))+`"`)
	}
	// The pending member's character took the deadline away (RN-15).
	if at := et.h.pendingExpiresAt(et.campaign, et.pia.id); at != nil {
		t.Errorf("the pending member's deadline is %v after their character, want none", at)
	}
}

// TestCreateCharacterEditorRefusesBadFields: the refusals of the request's own fields
// that the order test does not show, each with the field the web marks.
func TestCreateCharacterEditorRefusesBadFields(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	bg := et.master.addEntry(t, et.campaign, testBackground("Guarda de farol"))
	et.master.setOff(t, et.campaign, true, bg.GetKey())
	npc := func(sheet *charactersv1.CharacterSheet) *charactersv1.CreateCharacterRequest {
		return &charactersv1.CreateCharacterRequest{CampaignId: et.campaign, Kind: editorMinion, Name: "Goblin", Sheet: sheet}
	}
	pc := func(edit func(r *charactersv1.CreateCharacterRequest)) *charactersv1.CreateCharacterRequest {
		r := &charactersv1.CreateCharacterRequest{CampaignId: et.campaign, Kind: editorPlayer, Name: "Bruna", Sheet: editorFighter()}
		edit(r)
		return r
	}
	image := uuid.New().String()
	tests := []struct {
		name string
		as   *user
		req  *charactersv1.CreateCharacterRequest
		want string
	}{
		{"no sheet", et.dona, pc(func(r *charactersv1.CreateCharacterRequest) { r.Sheet = nil }), `invalid_argument: sheet is required: a full or a basic sheet | InvalidField{field="sheet"}`},
		{"a basic sheet for a player", et.dona, pc(func(r *charactersv1.CreateCharacterRequest) { r.Sheet = basicSheet() }), `invalid_argument: sheet must be a full sheet for this kind of character | InvalidField{field="sheet"}`},
		{"a full sheet for a minion", et.master, npc(enemySheet()), `invalid_argument: sheet must be a basic sheet for this kind of character | InvalidField{field="sheet"}`},
		{"XP for a player", et.dona, pc(func(r *charactersv1.CreateCharacterRequest) { r.Sheet.GetFull().XpValue = 10 }), `invalid_argument: sheet.full.xp_value must be 0 for a player's character | InvalidField{field="sheet.full.xp_value"}`},
		{"a name with a line break", et.dona, pc(func(r *charactersv1.CreateCharacterRequest) { r.Name = "a\nb" }), `invalid_argument: name must not contain the character U+000A | InvalidField{field="name"}`},
		{"a switched-off background for a player", et.dona, pc(func(r *charactersv1.CreateCharacterRequest) {
			r.Sheet.GetFull().Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: bg.GetKey()}
		}), `failed_precondition: the master switched off an option this sheet picks: it is not a new choice for players | CharacterBlocked{reason=CHARACTER_BLOCKED_REASON_SWITCHED_OFF_CONTENT content_key="` + bg.GetKey() + `"}`},
		{"a portrait with the gallery off", et.master, npc(withPortrait(basicSheet(), image)), `invalid_argument: sheet.basic.portrait_image_id cannot be set: the gallery is off | InvalidField{field="sheet.basic.portrait_image_id"}`},
		{"a portrait that is no ID", et.master, npc(withPortrait(basicSheet(), "retrato.png")), `invalid_argument: sheet.basic.portrait_image_id must be an image of the campaign's gallery | InvalidField{field="sheet.basic.portrait_image_id"}`},
		{"a line break in the eyes", et.dona, pc(func(r *charactersv1.CreateCharacterRequest) {
			r.Story = &charactersv1.CharacterStory{Appearance: &charactersv1.Appearance{Eyes: "a\nb"}}
		}), `invalid_argument: story.appearance.eyes must not contain the character U+000A | InvalidField{field="story.appearance.eyes"}`},
		{"a key too long", et.dona, pc(func(r *charactersv1.CreateCharacterRequest) { r.IdempotencyKey = strings.Repeat("k", 65) }), `invalid_argument: idempotency_key must be 1 to 64 characters`},
	}
	for _, tt := range tests {
		_, err := editorCreate(tt.as, tt.req)
		wantEditorRefusal(t, tt.name, err, tt.want)
	}
	// The master may pick what is switched off for players.
	if _, err := editorCreate(et.master, &charactersv1.CreateCharacterRequest{CampaignId: et.campaign, Kind: editorPlayer, ForPlayer: true, Name: "Reserva", Sheet: editorClone(editorFighter(), func(f *charactersv1.FullSheet) {
		f.Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: bg.GetKey()}
	})}); err != nil {
		t.Errorf("the master's reserved character with the switched-off background: %v", err)
	}

	// With the gallery on: an image of another campaign is refused, one of the campaign
	// is kept.
	gal := &editorGallery{images: map[string][]string{et.campaign: {image}}}
	et.h.svc.SetGallery(gal)
	_, err := editorCreate(et.master, npc(withPortrait(basicSheet(), uuid.New().String())))
	wantEditorRefusal(t, "an image the gallery does not have", err, `invalid_argument: sheet.basic.portrait_image_id must be an image of the campaign's gallery | InvalidField{field="sheet.basic.portrait_image_id"}`)
	got, err := editorCreate(et.master, npc(withPortrait(basicSheet(), image)))
	if err != nil || portraitOf(got.GetSheet()) != image {
		t.Errorf("an image of the gallery: %v, %v; want it kept", portraitOf(got.GetSheet()), err)
	}
	// A gallery that fails is the server's failure.
	gal.failCheck.Store(true)
	_, err = editorCreate(et.master, npc(withPortrait(basicSheet(), image)))
	wantEditorRefusal(t, "a gallery that fails", err, `internal: internal error`)
	et.editorLogged(t, `"msg":"characters: cannot check a portrait"`, `the gallery is down`)
}

// Finding: CreateCharacter asks the gallery for the portrait (which, for a fog map's
// background, makes a copy of the image in a transaction of its own) before it checks the
// story and the idempotency key, and before its transaction refuses: a create refused for
// one of those leaves the copy in the gallery. UpdateCharacter prepares the copy only for
// a save that can happen. Frozen here, not fixed.
func TestCreateCharacterEditorAsksTheGalleryBeforeTheStory(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	image := uuid.New().String()
	gal := &editorGallery{images: map[string][]string{et.campaign: {image}}}
	et.h.svc.SetGallery(gal)
	_, err := editorCreate(et.master, &charactersv1.CreateCharacterRequest{
		CampaignId: et.campaign, Kind: editorMinion, Name: "Goblin", Sheet: withPortrait(basicSheet(), image),
		Story: &charactersv1.CharacterStory{Allies: strings.Repeat("x", maxAlliesLength+1)},
	})
	wantEditorRefusal(t, "a bad story", err, `invalid_argument: story.allies must be at most 2000 characters, got 2001 | InvalidField{field="story.allies"}`)
	if n := gal.looked.Load(); n != 1 {
		t.Errorf("the gallery was asked %d times for a create refused for its story, want 1 (as today)", n)
	}
}

// TestCreateCharacterEditorRefusalsInsideTheTransaction: the checks the transaction makes,
// in their order: the way the scores were made, the table's hit point rule, the multiclass
// prerequisites, one living character per player, and the campaign's cap.
func TestCreateCharacterEditorRefusalsInsideTheTransaction(t *testing.T) {
	t.Parallel()
	const capacity = 3
	et := newEditorTable(t, func(c *Config) { c.MaxCharactersPerCampaign = capacity })
	first := et.dona.createPensantus(t, et.campaign)
	setRules(t, et.master, et.campaign, func(r *campaignsv1.TableRules) {
		methods(true, false, false, false)(r)
		hitPointsRule(campaignsv1.HitPointsRule_HIT_POINTS_RULE_AVERAGE)(r)
	})
	sheet := pensantusSheet()
	full := sheet.GetFull()
	full.BaseScores = &rulesv1.AbilityScores{Strength: 12, Dexterity: 14, Constitution: 13, Intelligence: 15, Wisdom: 10, Charisma: 8}
	full.HitPoints = &charactersv1.HitPoints{Method: charactersv1.HitPointsMethod_HIT_POINTS_METHOD_ROLLED, Rolls: []int32{4, 4, 4}}
	full.Classes = append(full.Classes, &charactersv1.ClassLevel{ClassKey: "class:barbarian", Level: 1})
	req := &charactersv1.CreateCharacterRequest{
		CampaignId: et.campaign, Kind: editorPlayer, Name: "Pensantus", Sheet: sheet,
		AbilityMethod: charactersv1.AbilityMethod_ABILITY_METHOD_TYPED,
	}
	steps := []struct {
		want string
		fix  func()
	}{
		{`failed_precondition: the ability scores are refused: ABILITY_SCORES_REFUSAL_REASON_METHOD_NOT_ALLOWED | AbilityScoresRefusal{reason=ABILITY_SCORES_REFUSAL_REASON_METHOD_NOT_ALLOWED method=ABILITY_METHOD_TYPED}`, func() { req.AbilityMethod = 0 }},
		{`failed_precondition: the ability scores are refused: ABILITY_SCORES_REFUSAL_REASON_METHOD_NOT_ALLOWED | AbilityScoresRefusal{reason=ABILITY_SCORES_REFUSAL_REASON_METHOD_NOT_ALLOWED}`, func() {
			req.AbilityMethod = charactersv1.AbilityMethod_ABILITY_METHOD_STANDARD_ARRAY
			full.BaseScores.Charisma = 9
		}},
		{`failed_precondition: the ability scores are refused: ABILITY_SCORES_REFUSAL_REASON_NOT_STANDARD_ARRAY | AbilityScoresRefusal{reason=ABILITY_SCORES_REFUSAL_REASON_NOT_STANDARD_ARRAY method=ABILITY_METHOD_STANDARD_ARRAY}`, func() {
			full.BaseScores.Charisma = 8
			full.ExtraAbilityBonuses = &rulesv1.AbilityScores{Strength: 3}
		}},
		{`failed_precondition: the ability scores are refused: ABILITY_SCORES_REFUSAL_REASON_EXTRA_BONUSES | AbilityScoresRefusal{reason=ABILITY_SCORES_REFUSAL_REASON_EXTRA_BONUSES method=ABILITY_METHOD_STANDARD_ARRAY}`, func() {
			full.ExtraAbilityBonuses = nil
		}},
		{`failed_precondition: the level up does not allow this choice: full.hit_points.method | LevelUpRefusal{field="full.hit_points.method" reason=LEVEL_UP_REFUSAL_REASON_HIT_POINTS_RULE}`, func() {
			full.HitPoints = &charactersv1.HitPoints{Method: charactersv1.HitPointsMethod_HIT_POINTS_METHOD_AVERAGE}
		}},
		{`failed_precondition: the level up does not allow this choice: sheet.full.classes[1].class_key | LevelUpRefusal{field="sheet.full.classes[1].class_key" reason=LEVEL_UP_REFUSAL_REASON_MULTICLASS_PREREQUISITE class_key="class:barbarian" ability=ABILITY_STRENGTH minimum=13 have=12}`, func() {
			full.Classes = full.Classes[:1]
		}},
		{`failed_precondition: you already have a living character in this campaign | CharacterBlocked{reason=CHARACTER_BLOCKED_REASON_LIVING_CHARACTER_EXISTS character_id="` + first.GetId() + `"}`, nil},
	}
	for i, step := range steps {
		_, err := editorCreate(et.dona, req)
		wantEditorRefusal(t, "step "+strconv.Itoa(i), err, step.want)
		if step.fix != nil {
			step.fix()
		}
	}
	// The cap: a campaign full with Dona's character and two NPCs refuses Caio's, and
	// still answers Dona with her living character (that check comes first).
	et.master.create(t, et.campaign, editorMinion, "Goblin", basicSheet())
	et.master.create(t, et.campaign, editorMinion, "Goblin 2", basicSheet())
	_, err := editorCreate(et.caio, req)
	wantEditorRefusal(t, "Caio in a full campaign", err, `resource_exhausted: the campaign already has 3 characters and NPCs, the most it may have; delete one to create another`)
	_, err = editorCreate(et.dona, req)
	wantEditorRefusal(t, "Dona in a full campaign", err, `failed_precondition: you already have a living character in this campaign | CharacterBlocked{reason=CHARACTER_BLOCKED_REASON_LIVING_CHARACTER_EXISTS character_id="`+first.GetId()+`"}`)
	_, err = editorCreate(et.master, &charactersv1.CreateCharacterRequest{CampaignId: et.campaign, Kind: editorMinion, Name: "Goblin 3", Sheet: basicSheet()})
	wantEditorRefusal(t, "an NPC in a full campaign", err, `resource_exhausted: the campaign already has 3 characters and NPCs, the most it may have; delete one to create another`)
	// The reserved character for a player skips the player's checks, not the cap.
	_, err = editorCreate(et.master, &charactersv1.CreateCharacterRequest{CampaignId: et.campaign, Kind: editorPlayer, ForPlayer: true, Name: "Reserva", Sheet: sheet})
	wantEditorRefusal(t, "a reserved character in a full campaign", err, `resource_exhausted: the campaign already has 3 characters and NPCs, the most it may have; delete one to create another`)
}

// TestCreateCharacterEditorRecordsTheOrigin: the server records how the scores were made,
// whatever the client sends, and the 4d6 the sheet used are spent.
func TestCreateCharacterEditorRecordsTheOrigin(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, func(c *Config) { c.Roller = nil })
	sheet := pensantusSheet()
	sheet.GetFull().AbilityOrigin = &charactersv1.AbilityOrigin{Method: charactersv1.AbilityMethod_ABILITY_METHOD_POINT_BUY}
	got, err := editorCreate(et.dona, &charactersv1.CreateCharacterRequest{CampaignId: et.campaign, Kind: editorPlayer, Name: "Pensantus", Sheet: sheet})
	if err != nil || got.GetSheet().GetFull().GetAbilityOrigin() != nil {
		t.Errorf("a client's origin with no method: %v, %v; want none recorded", got.GetSheet().GetFull().GetAbilityOrigin(), err)
	}
	typed := []int32{6, 5, 4, 1}
	if _, err := et.caio.rollScores(et.campaign, typed, typed, typed, typed, typed, typed); err != nil {
		t.Fatalf("RollAbilityScores() error = %v", err)
	}
	rolled := scoresSheet(15, 15, 15, 15, 15, 15)
	got, err = editorCreate(et.caio, &charactersv1.CreateCharacterRequest{
		CampaignId: et.campaign, Kind: editorPlayer, Name: "Pensantus", Sheet: rolled, AbilityMethod: charactersv1.AbilityMethod_ABILITY_METHOD_ROLLED_4D6,
	})
	if err != nil {
		t.Fatalf("CreateCharacter(rolled) error = %v", err)
	}
	if o := editorMessage(got.GetSheet().GetFull().GetAbilityOrigin().ProtoReflect()); !strings.HasPrefix(o, "AbilityOrigin{method=ABILITY_METHOD_ROLLED_4D6 rolls=[AbilityRollSet{dice=[6, 5, 4, 1] total=15}") || !strings.HasSuffix(o, "typed=true}") {
		t.Errorf("the origin recorded = %s, want the rolled 4d6 with the typed sets", o)
	}
	if stored := et.caio.storedScores(t, et.campaign); stored != nil {
		t.Errorf("the 4d6 are still stored after the sheet used them: %v", stored)
	}
}

// TestCreateCharacterEditorIdempotency: the key of a create is the caller's; the same
// request with it answers the first character, another request with it is refused.
func TestCreateCharacterEditorIdempotency(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	key := uuid.New().String()
	req := &charactersv1.CreateCharacterRequest{CampaignId: et.campaign, Kind: editorEnemy, Name: "Bandido", Sheet: enemySheet(), IdempotencyKey: key}
	first, err := editorCreate(et.master, req)
	if err != nil {
		t.Fatalf("CreateCharacter() error = %v", err)
	}
	again, err := editorCreate(et.master, req)
	if err != nil || again.GetId() != first.GetId() {
		t.Errorf("the same request again = %v, %v; want the first character", again.GetId(), err)
	}
	other := proto.CloneOf(req)
	other.Name = "Outro"
	_, err = editorCreate(et.master, other)
	wantEditorRefusal(t, "another request with the key", err, `invalid_argument: idempotency_key was used for another change`)
	// Another caller's identical key is another key.
	pc := &charactersv1.CreateCharacterRequest{CampaignId: et.campaign, Kind: editorPlayer, Name: "Pensantus", Sheet: pensantusSheet(), IdempotencyKey: key}
	mine, err := editorCreate(et.dona, pc)
	if err != nil || mine.GetId() == first.GetId() {
		t.Errorf("Dona's create with the master's key = %v, %v; want a character of her own", mine.GetId(), err)
	}
	// A pending member's replay answers the first character too (the deadline went with it).
	pending := proto.CloneOf(pc)
	pending.IdempotencyKey = uuid.New().String()
	pia, err := editorCreate(et.pia, pending)
	if err != nil {
		t.Fatalf("the pending member's create: %v", err)
	}
	piaAgain, err := editorCreate(et.pia, pending)
	if err != nil || piaAgain.GetId() != pia.GetId() {
		t.Errorf("the pending member's replay = %v, %v; want the first character", piaAgain.GetId(), err)
	}
	if n := et.editorCount(t); n != 3 {
		t.Errorf("the campaign has %d characters, want 3", n)
	}
}

// TestCreateCharacterEditorChecksAMovedContentAgain: a table entry written between the
// handler's read of the content and its transaction is seen by the transaction: the sheet
// is checked again with it and saved with its revision.
func TestCreateCharacterEditorChecksAMovedContentAgain(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	var added *rulesv1.TableEntry
	et.src.mu.Lock()
	et.src.hook = func() { added = et.master.addEntry(t, et.campaign, testBackground("Guarda de farol")) }
	et.src.mu.Unlock()
	got, err := editorCreate(et.dona, &charactersv1.CreateCharacterRequest{CampaignId: et.campaign, Kind: editorPlayer, Name: "Pensantus", Sheet: pensantusSheet()})
	if err != nil {
		t.Fatalf("CreateCharacter() error = %v", err)
	}
	if added == nil {
		t.Fatal("the hook did not run")
	}
	if rev := got.GetSheet().GetFull().GetContentRevision(); rev != 1 {
		t.Errorf("the sheet's content revision = %d, want 1: the one the transaction read", rev)
	}
	// An NPC too.
	et.src.mu.Lock()
	et.src.hook = func() { et.master.addEntry(t, et.campaign, testBackground("Outro fundo")) }
	et.src.mu.Unlock()
	npc, err := editorCreate(et.master, &charactersv1.CreateCharacterRequest{CampaignId: et.campaign, Kind: editorEnemy, Name: "Bandido", Sheet: enemySheet()})
	if err != nil || npc.GetSheet().GetFull().GetContentRevision() != 2 {
		t.Errorf("the NPC's content revision = %d, %v; want 2", npc.GetSheet().GetFull().GetContentRevision(), err)
	}
}

// TestCreateCharacterEditorWhenTheContentFails: a content that cannot be read, before the
// transaction or inside it, is the server's failure, logged with what failed.
func TestCreateCharacterEditorWhenTheContentFails(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	req := &charactersv1.CreateCharacterRequest{CampaignId: et.campaign, Kind: editorPlayer, Name: "Pensantus", Sheet: pensantusSheet()}
	et.src.failOutside.Store(true)
	_, err := editorCreate(et.dona, req)
	wantEditorRefusal(t, "the first read fails", err, `internal: internal error`)
	et.editorLogged(t, `"msg":"characters: cannot read rules content"`, `the content source is down`)
	et.src.failOutside.Store(false)
	et.src.failInside.Store(true)
	_, err = editorCreate(et.dona, req)
	wantEditorRefusal(t, "the read in the transaction fails", err, `internal: internal error`)
	et.editorLogged(t, `"msg":"characters: cannot create a character"`, `read the table content and rules: the content source is down`)
	if n := et.editorCount(t); n != 0 {
		t.Errorf("the failed creates left %d characters", n)
	}
}
