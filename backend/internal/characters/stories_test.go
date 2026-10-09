package characters

import (
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// Acceptance tests: one per acceptance criterion in
// docs/product/stories.md and per rule in docs/product/rules.md, named
// after the story or the rule. A story's backend part is done when its
// tests pass.

// MR-003 (the character half): Dado um convite válido para "Mirathel",
// quando o jogador entra, então cria um personagem do tipo jogador, que o
// mestre já vê na campanha.
func TestMR003_PlayerCreatesTheirCharacterAndTheMasterSeesIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, jogador := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(mestre, "Mirathel", jogador)

	created := jogador.createPensantus(t, campaign)
	if created.GetKind() != charactersv1.CharacterKind_CHARACTER_KIND_PLAYER || created.GetState() != charactersv1.CharacterState_CHARACTER_STATE_DRAFT ||
		created.GetPlayerUserId() != jogador.id || created.GetPlayerDisplayName() != "Jogadora" || created.GetRevision() != 1 ||
		created.GetCampaignId() != campaign || !created.GetCanEdit() {
		t.Fatalf("CreateCharacter() = %v, want the player's editable draft, revision 1", created)
	}

	list := mestre.list(t, campaign)
	if len(list) != 1 || list[0].GetId() != created.GetId() || list[0].GetName() != "Pensantus" ||
		list[0].GetPlayerDisplayName() != "Jogadora" || list[0].GetClassSummary() != "Mago 3" ||
		list[0].GetRaceNamePt() != "Gnomo das Rochas" || list[0].GetState() != charactersv1.CharacterState_CHARACTER_STATE_DRAFT {
		t.Errorf("master's ListCharacters() = %v, want Pensantus, Mago 3, Gnomo das Rochas, Jogadora's draft", list)
	}
	if got := mestre.get(t, campaign, created.GetId()); got.GetName() != "Pensantus" || !got.GetCanMarkDead() {
		t.Errorf("master's GetCharacter() = %v", got)
	}
}

// MR-004: Dado um personagem completo, quando o jogador abre a ficha, então
// vê as seções da ficha oficial e os valores calculados, como modificadores
// e CD de magia, vêm prontos do servidor.
func TestMR004_SheetComesWithServerCalculatedValues(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, jogador := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(mestre, "Mirathel", jogador)
	created := jogador.createPensantus(t, campaign)

	c := jogador.get(t, campaign, created.GetId())
	// The sheet comes back as the player filled it in (the choices), and the
	// numbers come from the server (ADR-0008 table for Pensantus).
	if !proto.Equal(c.GetSheet(), created.GetSheet()) || c.GetSheet().GetFull().GetRaceKey() != "race:gnome" {
		t.Errorf("GetCharacter() sheet = %v, want the one saved", c.GetSheet())
	}
	d := c.GetDerived()
	intelligence := d.GetAbilities()[3]
	if intelligence.GetAbility() != rulesv1.Ability_ABILITY_INTELLIGENCE || intelligence.GetScore() != 18 || intelligence.GetModifier() != 4 {
		t.Errorf("INT = %v, want 18 (+4)", intelligence)
	}
	sc := d.GetSpellcasting()
	if len(sc) != 1 || sc[0].GetSaveDc() != 14 || sc[0].GetAttackBonus() != 6 {
		t.Errorf("spellcasting = %v, want DC 14 and +6", sc)
	}
	if d.GetArmorClass() != 13 || d.GetHitPointsMax() != 23 || d.GetSpeedWalkFt() != 25 || d.GetProficiencyBonus() != 2 {
		t.Errorf("AC %d, HP %d, speed %d, proficiency %d; want 13, 23, 25, +2",
			d.GetArmorClass(), d.GetHitPointsMax(), d.GetSpeedWalkFt(), d.GetProficiencyBonus())
	}
	// Every section of the official sheet has its data: abilities, skills,
	// combat, spells, features (the equipment is the inventory's: inventory_test.go).
	sections := map[string]int{
		"habilidades": len(d.GetAbilities()), "perícias": len(d.GetSkills()), "ataques": len(d.GetAttacks()),
		"magias": len(d.GetSpells()), "características": len(d.GetFeatures()),
	}
	for section, n := range sections {
		if n == 0 {
			t.Errorf("section %s is empty", section)
		}
	}
	if len(d.GetIssues()) != 0 {
		t.Errorf("issues = %v, want none for Pensantus", d.GetIssues())
	}
}

// MR-005, first criterion: Dado que sou mestre de "Mirathel", quando crio um
// inimigo ou um boss, então ele tem ficha completa; quando crio um minion ou
// um NPC de história, então ele tem ficha básica.
func TestMR005_MasterCreatesNpcsOfEachKind(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, jogador := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(mestre, "Mirathel", jogador)
	pc := jogador.createPensantus(t, campaign)

	full := []charactersv1.CharacterKind{charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, charactersv1.CharacterKind_CHARACTER_KIND_BOSS}
	basic := []charactersv1.CharacterKind{charactersv1.CharacterKind_CHARACTER_KIND_MINION, charactersv1.CharacterKind_CHARACTER_KIND_STORY}
	for _, kind := range full {
		npc := mestre.create(t, campaign, kind, "Capitão "+kind.String(), enemySheet())
		if npc.GetSheet().GetFull() == nil || npc.GetDerived() == nil || npc.GetDerived().GetArmorClass() != 18 ||
			npc.GetPlayerUserId() != "" || npc.GetState() != charactersv1.CharacterState_CHARACTER_STATE_DRAFT {
			t.Errorf("%v = %v, want a full sheet with derived numbers (AC 18: chain mail and shield)", kind, npc)
		}
		// A full sheet is refused for a basic kind, and the other way round.
		_, err := mestre.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: campaign, Kind: kind, Name: "Errado", Sheet: basicSheet(),
		}))
		wantCode(t, "CreateCharacter("+kind.String()+", basic sheet)", err, connect.CodeInvalidArgument)
	}
	for _, kind := range basic {
		npc := mestre.create(t, campaign, kind, "Goblin "+kind.String(), basicSheet())
		if npc.GetSheet().GetBasic().GetHitPointsMax() != 7 || npc.GetDerived() != nil {
			t.Errorf("%v = %v, want a basic sheet without derived numbers", kind, npc)
		}
		if b := npc.GetSheet().GetBasic(); b.GetInitiativeBonus() != 2 || len(b.GetAttacks()) != 1 || b.GetAttacks()[0].GetName() != "Cimitarra" {
			t.Errorf("%v basic sheet = %v, want initiative +2 and the Cimitarra attack back as saved", kind, b)
		}
		// A sheet stored before Etapa 6 (a bonus and the damage as text) reads as an attack.
		if _, err := h.pool.Exec(t.Context(),
			`UPDATE characters SET sheet = '{"basic":{"hit_points_max":7,"armor_class":15,"attack_bonus":4,"damage":"1d6+2 cortante"}}' WHERE id = $1`,
			npc.GetId()); err != nil {
			t.Fatal(err)
		}
		if old := mestre.get(t, campaign, npc.GetId()).GetSheet().GetBasic(); len(old.GetAttacks()) != 1 || old.GetAttacks()[0].GetDamageDiceSides() != 6 || old.GetDamage() != "" {
			t.Errorf("%v old sheet = %v, want the old damage converted to one attack", kind, old)
		}
		_, err := mestre.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: campaign, Kind: kind, Name: "Errado", Sheet: enemySheet(),
		}))
		wantCode(t, "CreateCharacter("+kind.String()+", full sheet)", err, connect.CodeInvalidArgument)
	}

	// The master's list: the player's character first, then the NPCs.
	list := mestre.list(t, campaign)
	if len(list) != 5 || list[0].GetId() != pc.GetId() {
		t.Fatalf("master's ListCharacters() = %v, want Pensantus and then 4 NPCs", list)
	}
	if list[1].GetClassSummary() != "Guerreiro 2" || list[3].GetClassSummary() != "" {
		t.Errorf("class summaries = %q, %q; want Guerreiro 2 for a full NPC, empty for a basic one", list[1].GetClassSummary(), list[3].GetClassSummary())
	}
}

// MR-005, second criterion: Dado que sou jogador de "Mirathel", quando abro
// a campanha, então não vejo nenhum NPC e o servidor recusa se eu tentar
// criar um. Third criterion: um NPC criado em "Mirathel" não aparece em
// outra campanha do mestre (reusing NPCs is MR-022).
func TestMR005_PlayersCannotSeeOrCreateNpcs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, jogador := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(mestre, "Mirathel", jogador)
	other := h.newCampaign(mestre, "Outra", jogador)
	pc := jogador.createPensantus(t, campaign)
	boss := mestre.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_BOSS, "Strahd", enemySheet())
	minion := mestre.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin", basicSheet())

	if list := jogador.list(t, campaign); len(list) != 1 || list[0].GetId() != pc.GetId() {
		t.Errorf("player's ListCharacters() = %v, want only their own character", list)
	}
	for _, npc := range []*charactersv1.Character{boss, minion} {
		_, err := jogador.api.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: campaign, CharacterId: npc.GetId()}))
		wantCode(t, "player's GetCharacter(NPC)", err, connect.CodeNotFound)
	}
	for _, kind := range []charactersv1.CharacterKind{
		charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, charactersv1.CharacterKind_CHARACTER_KIND_BOSS,
		charactersv1.CharacterKind_CHARACTER_KIND_MINION, charactersv1.CharacterKind_CHARACTER_KIND_STORY,
	} {
		_, err := jogador.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
			CampaignId: campaign, Kind: kind, Name: "Meu NPC", Sheet: basicSheet(),
		}))
		wantCode(t, "player's CreateCharacter("+kind.String()+")", err, connect.CodePermissionDenied)
	}

	// The NPCs belong to Mirathel only.
	if list := mestre.list(t, other); len(list) != 0 {
		t.Errorf("master's ListCharacters(other campaign) = %v, want none", list)
	}
	_, err := mestre.api.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: other, CharacterId: boss.GetId()}))
	wantCode(t, "GetCharacter(boss, other campaign)", err, connect.CodeNotFound)
}

// MR-006, second criterion: Dado que nenhuma sessão começou, quando o
// jogador edita a ficha, então a alteração é salva.
func TestMR006_BeforeAnySessionThePlayerEditsTheSheet(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, jogador := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(mestre, "Mirathel", jogador)
	c := jogador.createPensantus(t, campaign)

	sheet := pensantusSheet()
	sheet.GetFull().BaseScores.Intelligence = 17
	updated, err := jogador.update(t, c, "Pensantus, o Sábio", sheet)
	if err != nil {
		t.Fatalf("player's UpdateCharacter() before any session error = %v", err)
	}
	if updated.GetName() != "Pensantus, o Sábio" || updated.GetRevision() != 2 ||
		updated.GetSheet().GetFull().GetBaseScores().GetIntelligence() != 17 || updated.GetDerived().GetAbilities()[3].GetScore() != 19 {
		t.Errorf("UpdateCharacter() = %v, want the new name and INT, revision 2, derived INT 19", updated)
	}
	if got := jogador.get(t, campaign, c.GetId()); got.GetRevision() != 2 || got.GetName() != "Pensantus, o Sábio" {
		t.Errorf("GetCharacter() after the update = %v, want it saved", got)
	}
}

// MR-006, first criterion: Dado que a primeira sessão da campanha já
// começou, quando o jogador tenta editar as habilidades da própria ficha,
// então o servidor recusa e o mestre consegue editar a mesma ficha.
func TestMR006_AfterTheFirstSessionOnlyTheMasterEditsTheSheet(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, jogador := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(mestre, "Mirathel", jogador)
	c := jogador.createPensantus(t, campaign)

	// Given: the first session starts (package play does this).
	if n := h.lockSheets(campaign); n != 1 {
		t.Fatalf("LockSheets() = %d, want 1", n)
	}

	c = jogador.get(t, campaign, c.GetId())
	if c.GetState() != charactersv1.CharacterState_CHARACTER_STATE_LOCKED || c.GetCanEdit() || c.GetSheetLockedAt() == nil {
		t.Errorf("player's GetCharacter() = %v, want locked, not editable, with sheet_locked_at", c)
	}
	sheet := pensantusSheet()
	sheet.GetFull().BaseScores.Intelligence = 20
	_, err := jogador.update(t, c, "Pensantus", sheet)
	if d := blocked(t, "player's UpdateCharacter() after the lock", err); d.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_SHEET_LOCKED || d.GetCharacterId() != c.GetId() {
		t.Errorf("CharacterBlocked = %v, want SHEET_LOCKED for this character", d)
	}

	updated, err := mestre.update(t, c, "Pensantus", sheet)
	if err != nil {
		t.Fatalf("master's UpdateCharacter() after the lock error = %v", err)
	}
	if updated.GetSheet().GetFull().GetBaseScores().GetIntelligence() != 20 || updated.GetState() != charactersv1.CharacterState_CHARACTER_STATE_LOCKED {
		t.Errorf("master's UpdateCharacter() = %v, want INT 20 and still locked", updated)
	}
}

// MR-006, third criterion: Dado um personagem criado depois da primeira
// sessão, quando o jogador edita a ficha antes da próxima sessão, então a
// alteração é salva e, quando a próxima sessão começa, a ficha trava.
func TestMR006_CharacterCreatedAfterTheFirstSessionLocksAtTheNextOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, jogador := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(mestre, "Mirathel", jogador)
	h.lockSheets(campaign) // the first session, before anyone had a character

	c := jogador.createPensantus(t, campaign)
	if c.GetState() != charactersv1.CharacterState_CHARACTER_STATE_DRAFT {
		t.Fatalf("a character created after the first session is %v, want a draft", c.GetState())
	}
	c, err := jogador.update(t, c, "Pensantus", pensantusSheet())
	if err != nil {
		t.Fatalf("player's UpdateCharacter() before the next session error = %v", err)
	}

	h.lockSheets(campaign) // the next session
	_, err = jogador.update(t, c, "Pensantus", pensantusSheet())
	if d := blocked(t, "player's UpdateCharacter() after the next session", err); d.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_SHEET_LOCKED {
		t.Errorf("CharacterBlocked = %v, want SHEET_LOCKED", d)
	}
}

// MR-006, fourth criterion: Dado que a ficha travou, quando o jogador tenta
// editar a história do personagem, então o servidor recusa; depois que o
// mestre libera a história desse personagem, o jogador edita e salva, e a
// liberação acaba quando a próxima sessão começa.
func TestMR006_AfterTheLockTheStoryNeedsTheMastersPermission(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, jogador := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(mestre, "Mirathel", jogador)
	c := jogador.createPensantus(t, campaign)
	story := &charactersv1.CharacterStory{Backstory: "Nasceu em Mirathel."}

	// A draft's story is the player's to edit.
	c, err := jogador.updateStory(t, c, story)
	if err != nil || c.GetStory().GetBackstory() != "Nasceu em Mirathel." || c.GetRevision() != 2 {
		t.Fatalf("player's UpdateCharacterStory() on a draft = %v, %v; want it saved, revision 2", c, err)
	}

	h.lockSheets(campaign)
	c = jogador.get(t, campaign, c.GetId())
	if c.GetCanEditStory() || c.GetStoryEditingAllowed() {
		t.Errorf("after the lock can_edit_story = %v, story_editing_allowed = %v; want false, false", c.GetCanEditStory(), c.GetStoryEditingAllowed())
	}
	story.Backstory = "Nasceu em Mirathel e estudou em Waterdeep."
	_, err = jogador.updateStory(t, c, story)
	if d := blocked(t, "player's UpdateCharacterStory() after the lock", err); d.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_STORY_LOCKED {
		t.Errorf("CharacterBlocked = %v, want STORY_LOCKED", d)
	}

	// The master allows it, for this character only; the revision does not
	// move.
	allowed := mestre.setStoryEditing(t, c, true)
	if !allowed.GetStoryEditingAllowed() || allowed.GetRevision() != c.GetRevision() {
		t.Errorf("SetStoryEditing(true) = %v, want allowed and revision %d", allowed, c.GetRevision())
	}
	c = jogador.get(t, campaign, c.GetId())
	if !c.GetCanEditStory() || c.GetCanEdit() {
		t.Errorf("player may edit the story %v and the sheet %v; want the story only", c.GetCanEditStory(), c.GetCanEdit())
	}
	c, err = jogador.updateStory(t, c, story)
	if err != nil || c.GetStory().GetBackstory() != story.GetBackstory() {
		t.Fatalf("player's UpdateCharacterStory() while allowed = %v, %v; want it saved", c, err)
	}

	// The permission ends when the next session starts.
	h.lockSheets(campaign)
	c = jogador.get(t, campaign, c.GetId())
	if c.GetStoryEditingAllowed() {
		t.Error("story_editing_allowed survived the next session")
	}
	_, err = jogador.updateStory(t, c, story)
	blocked(t, "player's UpdateCharacterStory() after the next session", err)

	// The master may also take it back, and always edits the story.
	mestre.setStoryEditing(t, c, true)
	c = mestre.setStoryEditing(t, c, false)
	_, err = jogador.updateStory(t, c, story)
	blocked(t, "player's UpdateCharacterStory() after the master took it back", err)
	if _, err := mestre.updateStory(t, c, story); err != nil {
		t.Errorf("master's UpdateCharacterStory() on a locked character error = %v", err)
	}
}

// RN-03: dentro da campanha, o jogador só cria um personagem novo quando o
// atual morre. Also when the calls race: the unique index decides.
func TestRN03_OneLivingCharacterPerPlayerPerCampaign(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 8) // the racers must overlap: one connection would run them one by one
	h := newHarness(t)
	mestre, jogador, outra := h.newUser("Mestre"), h.newUser("Jogadora"), h.newUser("Outra")
	campaign := h.newCampaign(mestre, "Mirathel", jogador, outra)
	other := h.newCampaign(mestre, "Outra campanha", jogador)
	first := jogador.createPensantus(t, campaign)

	_, err := jogador.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Segundo", Sheet: pensantusSheet(),
	}))
	d := blocked(t, "a second living character", err)
	if d.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_LIVING_CHARACTER_EXISTS || d.GetCharacterId() != first.GetId() {
		t.Errorf("CharacterBlocked = %v, want LIVING_CHARACTER_EXISTS with the living character's ID", d)
	}
	// Other players, and the same player in another campaign, are not
	// affected.
	outra.createPensantus(t, campaign)
	jogador.createPensantus(t, other)

	// Several creations at once, from a player with no character yet: one
	// wins, the others get the same clear answer.
	racer := h.newUser("Apressada")
	h.join(mestre, campaign, racer)
	var wg sync.WaitGroup
	results := make([]error, 5)
	start := dbtest.NewBarrier(len(results))
	for i := range results {
		wg.Go(func() {
			start.Wait()
			_, results[i] = racer.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
				CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Rápida", Sheet: pensantusSheet(),
			}))
		})
	}
	wg.Wait()
	created := 0
	for _, err := range results {
		if err == nil {
			created++
			continue
		}
		if d := blocked(t, "a racing CreateCharacter", err); d.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_LIVING_CHARACTER_EXISTS {
			t.Errorf("racing CreateCharacter() blocked for %v, want LIVING_CHARACTER_EXISTS", d.GetReason())
		}
	}
	if created != 1 {
		t.Errorf("%d of 5 racing CreateCharacter() calls succeeded, want exactly 1", created)
	}
}

// RN-03: o personagem morto não é apagado: fica no sistema, e o jogador pode
// criar outro.
func TestRN03_DeadCharacterStaysAndAllowsANewOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, jogador := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(mestre, "Mirathel", jogador)
	first := jogador.createPensantus(t, campaign)

	dead := mestre.markDead(t, first)
	if dead.GetState() != charactersv1.CharacterState_CHARACTER_STATE_DEAD || dead.GetDiedAt() == nil || dead.GetCanMarkDead() ||
		dead.GetRevision() != first.GetRevision() {
		t.Fatalf("MarkCharacterDead() = %v, want dead, with died_at, same revision", dead)
	}
	h.clock.Advance(1)
	again := mestre.markDead(t, first)
	if !again.GetDiedAt().AsTime().Equal(dead.GetDiedAt().AsTime()) {
		t.Errorf("marking dead again moved died_at from %v to %v", dead.GetDiedAt().AsTime(), again.GetDiedAt().AsTime())
	}

	second := jogador.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Pensantus II", pensantusSheet())
	list := jogador.list(t, campaign)
	if len(list) != 2 || list[0].GetId() != first.GetId() || list[0].GetState() != charactersv1.CharacterState_CHARACTER_STATE_DEAD ||
		list[1].GetId() != second.GetId() {
		t.Errorf("player's ListCharacters() = %v, want the dead one and the new one", list)
	}

	// The dead character stays readable, and only the master edits it.
	d := jogador.get(t, campaign, first.GetId())
	_, err := jogador.update(t, d, "Pensantus", pensantusSheet())
	if b := blocked(t, "player's UpdateCharacter(dead)", err); b.GetReason() != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_CHARACTER_DEAD {
		t.Errorf("CharacterBlocked = %v, want CHARACTER_DEAD", b)
	}
	if _, err := mestre.update(t, d, "Pensantus (morto)", pensantusSheet()); err != nil {
		t.Errorf("master's UpdateCharacter(dead) error = %v", err)
	}

	// NPCs do not die (their hit points belong to each combat).
	npc := mestre.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin", basicSheet())
	_, err = mestre.api.MarkCharacterDead(t.Context(), connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: campaign, CharacterId: npc.GetId()}))
	wantCode(t, "MarkCharacterDead(NPC)", err, connect.CodeInvalidArgument)
}

// RN-11: só o mestre lê e edita as notas do mestre. They never reach a
// player: not in their character, not in their list, not in any error.
func TestRN11_PlayersNeverReceiveMasterNotes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, jogador := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(mestre, "Mirathel", jogador)
	c := jogador.createPensantus(t, campaign)
	const notes = "Pensantus é o filho perdido do lich"

	res, err := mestre.api.UpdateMasterNotes(t.Context(), connect.NewRequest(&charactersv1.UpdateMasterNotesRequest{
		CampaignId: campaign, CharacterId: c.GetId(), Notes: "  " + notes + "\n",
	}))
	if err != nil || res.Msg.GetNotes() != notes || res.Msg.GetUpdatedAt() == nil {
		t.Fatalf("UpdateMasterNotes() = %v, %v; want the trimmed notes", res, err)
	}
	got, err := mestre.api.GetMasterNotes(t.Context(), connect.NewRequest(&charactersv1.GetMasterNotesRequest{CampaignId: campaign, CharacterId: c.GetId()}))
	if err != nil || got.Msg.GetNotes() != notes {
		t.Fatalf("master's GetMasterNotes() = %v, %v", got, err)
	}

	// Everything the player can ask for, in JSON, never has the notes.
	var seen []string
	record := func(call string, m proto.Message, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("player's %s error = %v", call, err)
		}
		b, _ := protojson.Marshal(m)
		seen = append(seen, string(b))
	}
	gc, err := jogador.api.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: campaign, CharacterId: c.GetId()}))
	if err != nil {
		t.Fatalf("player's GetCharacter() error = %v", err)
	}
	record("GetCharacter()", gc.Msg, nil)
	lc, err := jogador.api.ListCharacters(t.Context(), connect.NewRequest(&charactersv1.ListCharactersRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatalf("player's ListCharacters() error = %v", err)
	}
	record("ListCharacters()", lc.Msg, nil)
	updated, err := jogador.update(t, c, "Pensantus", pensantusSheet())
	record("UpdateCharacter()", updated, err)
	storied, err := jogador.updateStory(t, updated, &charactersv1.CharacterStory{Allies: "A Ordem"})
	record("UpdateCharacterStory()", storied, err)
	for _, call := range []func() error{
		func() error {
			_, err := jogador.api.GetMasterNotes(t.Context(), connect.NewRequest(&charactersv1.GetMasterNotesRequest{CampaignId: campaign, CharacterId: c.GetId()}))
			return err
		},
		func() error {
			_, err := jogador.api.UpdateMasterNotes(t.Context(), connect.NewRequest(&charactersv1.UpdateMasterNotesRequest{CampaignId: campaign, CharacterId: c.GetId(), Notes: "x"}))
			return err
		},
	} {
		err := call()
		wantCode(t, "player's master notes call", err, connect.CodePermissionDenied)
		seen = append(seen, err.Error())
	}
	for _, body := range seen {
		if strings.Contains(body, "lich") {
			t.Errorf("a player's response carries the master's notes: %s", body)
		}
	}

	// Empty notes delete the row.
	if _, err := mestre.api.UpdateMasterNotes(t.Context(), connect.NewRequest(&charactersv1.UpdateMasterNotesRequest{
		CampaignId: campaign, CharacterId: c.GetId(), Notes: "   ",
	})); err != nil {
		t.Fatalf("UpdateMasterNotes(empty) error = %v", err)
	}
	var rows int
	if err := h.pool.QueryRow(t.Context(), "SELECT count(*) FROM character_master_notes").Scan(&rows); err != nil || rows != 0 {
		t.Errorf("character_master_notes after empty notes = %d rows, %v; want 0", rows, err)
	}
}

// RN-16: quando um jogador exclui a conta, os personagens dele ficam com o
// mestre da campanha, não apagados. The foreign keys do it: SET NULL for
// the player, CASCADE for a master's NPCs.
func TestRN16_DeletingAccountsKeepsPlayerCharacters(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mestre, jogador, outra := h.newUser("Mestre"), h.newUser("Jogadora"), h.newUser("Outra")
	campaign := h.newCampaign(mestre, "Mirathel", jogador, outra)
	pc := jogador.createPensantus(t, campaign)
	kept := outra.createPensantus(t, campaign)
	npc := mestre.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_BOSS, "Strahd", enemySheet())
	if _, err := mestre.api.UpdateMasterNotes(t.Context(), connect.NewRequest(&charactersv1.UpdateMasterNotesRequest{
		CampaignId: campaign, CharacterId: npc.GetId(), Notes: "vampiro",
	})); err != nil {
		t.Fatalf("UpdateMasterNotes() error = %v", err)
	}

	h.deleteUser(jogador.id)
	orphan := mestre.get(t, campaign, pc.GetId())
	if orphan.GetPlayerUserId() != "" || orphan.GetPlayerDisplayName() != "" || orphan.GetName() != "Pensantus" {
		t.Errorf("after the player deleted their account, the character = %v; want it kept, without its player", orphan)
	}
	if list := mestre.list(t, campaign); len(list) != 3 {
		t.Errorf("master's list after the player left = %d characters, want 3", len(list))
	}

	// The master's account takes the campaign with it (docs/privacy.md),
	// and the master's NPCs and notes; the other player's character stays,
	// without a campaign.
	h.deleteUser(mestre.id)
	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := h.pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return n
	}
	if n := count("SELECT count(*) FROM characters WHERE id = $1", npc.GetId()); n != 0 {
		t.Errorf("the master's NPC survived the master's account")
	}
	if n := count("SELECT count(*) FROM character_master_notes"); n != 0 {
		t.Errorf("%d master's notes survived the campaign", n)
	}
	if n := count("SELECT count(*) FROM characters WHERE id = $1 AND campaign_id IS NULL AND player_user_id = $2", kept.GetId(), outra.id); n != 1 {
		t.Errorf("the other player's character was not kept without a campaign")
	}
}
