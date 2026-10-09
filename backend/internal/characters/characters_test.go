package characters

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dbtest"
)

// TestStaleRevisionIsAborted: two people editing the same character cannot
// overwrite each other; the second save gets aborted (AIP-154) and changes
// nothing. A lock wins over a stale revision, because retrying would not
// help.
func TestStaleRevisionIsAborted(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	read := player.createPensantus(t, campaign) // revision 1, as the player's tab has it

	if _, err := master.update(t, read, "Pensantus do mestre", pensantusSheet()); err != nil {
		t.Fatalf("master's UpdateCharacter() error = %v", err)
	}
	_, err := player.update(t, read, "Pensantus da jogadora", pensantusSheet())
	wantCode(t, "UpdateCharacter() with a stale revision", err, connect.CodeAborted)
	_, err = player.updateStory(t, read, &charactersv1.CharacterStory{Backstory: "Outra história"})
	wantCode(t, "UpdateCharacterStory() with a stale revision", err, connect.CodeAborted)
	if got := master.get(t, campaign, read.GetId()); got.GetName() != "Pensantus do mestre" || got.GetRevision() != 2 || got.GetStory().GetBackstory() != "" {
		t.Errorf("after the stale saves the character = %v, want the master's, revision 2", got)
	}

	_, err = player.api.UpdateCharacter(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterRequest{
		CampaignId: campaign, CharacterId: read.GetId(), Revision: 0, Name: "X", Sheet: pensantusSheet(),
	}))
	wantCode(t, "UpdateCharacter(revision 0)", err, connect.CodeInvalidArgument)

	h.lockSheets(campaign)
	_, err = player.update(t, read, "Pensantus", pensantusSheet())
	blocked(t, "UpdateCharacter() locked and stale", err)
	_, err = player.updateStory(t, read, &charactersv1.CharacterStory{})
	blocked(t, "UpdateCharacterStory() locked and stale", err)
}

// TestUpdateCharacterValidation: a request that breaks a rule gets
// invalid_argument naming the field, never repeating what was typed.
func TestUpdateCharacterValidation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	c := player.createPensantus(t, campaign)
	const typed = "Segredo"

	tests := []struct {
		name  string
		edit  func(req *charactersv1.UpdateCharacterRequest)
		field string
	}{
		{"ability 31", func(r *charactersv1.UpdateCharacterRequest) { r.Sheet.GetFull().BaseScores.Intelligence = 31 }, "sheet.full.base_scores.intelligence"},
		{"empty name", func(r *charactersv1.UpdateCharacterRequest) { r.Name = "   " }, "name"},
		{"long name", func(r *charactersv1.UpdateCharacterRequest) { r.Name = typed + strings.Repeat("x", MaxNameLength) }, "name"},
		{"name with a line break", func(r *charactersv1.UpdateCharacterRequest) { r.Name = typed + "\nlinha" }, "name"},
		{"no sheet", func(r *charactersv1.UpdateCharacterRequest) { r.Sheet = nil }, "sheet"},
		{"a basic sheet for a player character", func(r *charactersv1.UpdateCharacterRequest) { r.Sheet = basicSheet() }, "sheet"},
		{"unknown subrace", func(r *charactersv1.UpdateCharacterRequest) { r.Sheet.GetFull().SubraceKey = "subrace:high-elf" }, "sheet.full.subrace_key"},
		{"item typed with a tab", func(r *charactersv1.UpdateCharacterRequest) {
			r.Sheet.GetFull().Equipment[0].Name = typed + "\tx"
		}, "sheet.full.equipment[0].name"},
	}
	for _, tt := range tests {
		req := &charactersv1.UpdateCharacterRequest{
			CampaignId: campaign, CharacterId: c.GetId(), Revision: c.GetRevision(), Name: "Pensantus", Sheet: pensantusSheet(),
		}
		tt.edit(req)
		_, err := player.api.UpdateCharacter(t.Context(), connect.NewRequest(req))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("%s: error = %v, want invalid_argument", tt.name, err)
			continue
		}
		if msg := err.Error(); !strings.Contains(msg, tt.field+" ") || strings.Contains(msg, typed) {
			t.Errorf("%s: message %q should name %s and never repeat what was typed", tt.name, msg, tt.field)
		}
	}
	_, err := master.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{CampaignId: campaign, Name: "Sem tipo", Sheet: basicSheet()}))
	wantCode(t, "CreateCharacter() without a kind", err, connect.CodeInvalidArgument)
	_, err = player.api.UpdateCharacterStory(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterStoryRequest{
		CampaignId: campaign, CharacterId: c.GetId(), Revision: c.GetRevision(),
		Story: &charactersv1.CharacterStory{Backstory: strings.Repeat("é", maxBackstoryLength+1)},
	}))
	wantCode(t, "UpdateCharacterStory() with a long backstory", err, connect.CodeInvalidArgument)
	_, err = master.api.UpdateMasterNotes(t.Context(), connect.NewRequest(&charactersv1.UpdateMasterNotesRequest{
		CampaignId: campaign, CharacterId: c.GetId(), Notes: strings.Repeat("x", MaxMasterNotesLength+1),
	}))
	wantCode(t, "UpdateMasterNotes() too long", err, connect.CodeInvalidArgument)

	if got := player.get(t, campaign, c.GetId()); got.GetRevision() != 1 {
		t.Errorf("revision after refused updates = %d, want 1", got.GetRevision())
	}
}

// TestSetStoryEditing: only for a player character, setting the same value
// changes nothing, and the revision never moves.
func TestSetStoryEditing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	c := player.createPensantus(t, campaign)
	npc := master.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_STORY, "Taverneiro", basicSheet())

	_, err := master.api.SetStoryEditing(t.Context(), connect.NewRequest(&charactersv1.SetStoryEditingRequest{CampaignId: campaign, CharacterId: npc.GetId(), Allowed: true}))
	wantCode(t, "SetStoryEditing(NPC)", err, connect.CodeInvalidArgument)

	once := master.setStoryEditing(t, c, true)
	twice := master.setStoryEditing(t, c, true)
	if !twice.GetStoryEditingAllowed() || twice.GetRevision() != c.GetRevision() || !twice.GetUpdatedAt().AsTime().Equal(c.GetUpdatedAt().AsTime()) {
		t.Errorf("SetStoryEditing(true) twice = %v, want allowed, with the revision and updated_at unchanged", twice)
	}
	if !once.GetCanSetStoryEditing() {
		t.Error("can_set_story_editing is false for the master")
	}
	if off := master.setStoryEditing(t, c, false); off.GetStoryEditingAllowed() {
		t.Errorf("SetStoryEditing(false) = %v, want not allowed", off)
	}
}

// TestMasterNotesBelongToTheCampaign: notes are per campaign and character,
// and a character of another campaign is "not found".
func TestMasterNotesBelongToTheCampaign(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	other := h.newCampaign(master, "Outra")
	npc := master.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_BOSS, "Strahd", enemySheet())

	res, err := master.api.GetMasterNotes(t.Context(), connect.NewRequest(&charactersv1.GetMasterNotesRequest{CampaignId: campaign, CharacterId: npc.GetId()}))
	if err != nil || res.Msg.GetNotes() != "" || res.Msg.GetUpdatedAt() != nil {
		t.Errorf("GetMasterNotes() with none written = %v, %v; want empty", res, err)
	}
	_, err = master.api.GetMasterNotes(t.Context(), connect.NewRequest(&charactersv1.GetMasterNotesRequest{CampaignId: other, CharacterId: npc.GetId()}))
	wantCode(t, "GetMasterNotes(character of another campaign)", err, connect.CodeNotFound)
	_, err = master.api.UpdateMasterNotes(t.Context(), connect.NewRequest(&charactersv1.UpdateMasterNotesRequest{CampaignId: other, CharacterId: npc.GetId(), Notes: "x"}))
	wantCode(t, "UpdateMasterNotes(character of another campaign)", err, connect.CodeNotFound)
	_, err = master.api.UpdateMasterNotes(t.Context(), connect.NewRequest(&charactersv1.UpdateMasterNotesRequest{CampaignId: campaign, CharacterId: "pensantus", Notes: "x"}))
	wantCode(t, "UpdateMasterNotes(not a UUID)", err, connect.CodeNotFound)
}

// TestListContent: any member gets the SRD catalog, with the attribution
// the credits page shows.
func TestListContent(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)

	res, err := player.content.ListContent(t.Context(), connect.NewRequest(&rulesv1.ListContentRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatalf("ListContent() error = %v", err)
	}
	c := res.Msg.GetContent()
	if c.GetContentVersion() != loadRules(t).Version() || !strings.Contains(c.GetAttribution(), "Creative Commons") ||
		len(c.GetAbilities()) != 6 || len(c.GetClasses()) != 12 || len(c.GetRaces()) != 9 || len(c.GetSpells()) == 0 {
		t.Errorf("ListContent() = version %q, %d abilities, %d classes, %d races, %d spells; want the SRD 5.1",
			c.GetContentVersion(), len(c.GetAbilities()), len(c.GetClasses()), len(c.GetRaces()), len(c.GetSpells()))
	}
}

// TestUniqueViolationIsRecognized: the 23505 that CockroachDB raises for
// the RN-03 index is the one the handler maps to LIVING_CHARACTER_EXISTS.
func TestUniqueViolationIsRecognized(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	player.createPensantus(t, campaign)

	_, err := h.pool.Exec(t.Context(), `
		INSERT INTO characters (campaign_id, kind, player_user_id, name, sheet, created_at, updated_at)
		VALUES ($1, 'player', $2, 'Segundo', '{}', now(), now())`, campaign, player.id)
	if !isUniqueViolation(err, "characters_one_living_player_character") {
		t.Errorf("second living character error = %v, want the characters_one_living_player_character violation", err)
	}
	if isUniqueViolation(err, "game_sessions_one_open_per_campaign") {
		t.Error("isUniqueViolation() matched another index")
	}
}

// TestOrphanedPlayerCharactersAreDeletedByTheDatabase: a player character
// with neither a player nor a campaign is expired for CockroachDB's
// row-level TTL job (migration 00020, docs/privacy.md), and no other
// row is. The job itself runs once a day, so the test reads the table's TTL
// setting and evaluates that same expression over real rows.
func TestOrphanedPlayerCharactersAreDeletedByTheDatabase(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	masterA, masterB := h.newUser("Mestre A"), h.newUser("Mestre B")
	stays, leaves, staysB, leavesB := h.newUser("Fica"), h.newUser("Sai"), h.newUser("Fica B"), h.newUser("Sai B")
	a := h.newCampaign(masterA, "Mirathel", stays, leaves)
	b := h.newCampaign(masterB, "Barovia", staysB, leavesB)
	kept := map[string]string{
		stays.createPensantus(t, a).GetId():  "a player character with its player and campaign",
		leaves.createPensantus(t, a).GetId(): "a player character whose player left (RN-16)",
		staysB.createPensantus(t, b).GetId(): "a player character whose campaign was deleted",
		masterA.create(t, a, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Goblin", basicSheet()).GetId(): "an NPC",
	}
	orphan := leavesB.createPensantus(t, b).GetId()
	h.deleteUser(leaves.id)
	h.deleteUser(masterB.id) // takes campaign b with it
	h.deleteUser(leavesB.id)

	var options []string
	if err := h.pool.QueryRow(t.Context(), "SELECT reloptions FROM pg_class WHERE relname = 'characters'").Scan(&options); err != nil {
		t.Fatalf("read the table's options: %v", err)
	}
	var ttlOn bool
	var expression string
	for _, o := range options {
		switch {
		case o == "ttl='on'":
			ttlOn = true
		case strings.HasPrefix(o, "ttl_expiration_expression="):
			// Stored as an escaped string literal: e'CASE WHEN kind = \'player\' ...'
			v := strings.TrimPrefix(o, "ttl_expiration_expression=")
			v = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(v, "e"), "'"), "'")
			expression = strings.ReplaceAll(v, `\'`, `'`)
		}
	}
	if !ttlOn || expression == "" {
		t.Fatalf("characters has no row-level TTL with an expiration expression: %q", options)
	}

	rows, err := h.pool.Query(t.Context(), fmt.Sprintf("SELECT id, (%s) FROM characters", expression))
	if err != nil {
		t.Fatalf("evaluate %q: %v", expression, err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var id string
		var expires *time.Time
		if err := rows.Scan(&id, &expires); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seen++
		switch {
		case id == orphan && (expires == nil || !expires.Before(time.Now())):
			t.Errorf("the orphaned character expires at %v, want a time in the past", expires)
		case id != orphan && expires != nil:
			t.Errorf("%s expires at %v, want never", kept[id], expires)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read rows: %v", err)
	}
	if seen != len(kept)+1 {
		t.Errorf("evaluated %d rows, want %d", seen, len(kept)+1)
	}
}

// TestGetSpellDetails: a member (and a pending member) gets one spell in
// full, structured; an unknown key is not_found; ListContent stays light.
func TestGetSpellDetails(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player, outsider := h.newUser("Mestre"), h.newUser("Jogadora"), h.newUser("De fora")
	campaign := h.newCampaign(master, "Mirathel", player)
	get := func(u *user, campaign, key string) (*rulesv1.SpellDetails, error) {
		res, err := u.content.GetSpellDetails(t.Context(), connect.NewRequest(&rulesv1.GetSpellDetailsRequest{CampaignId: campaign, SpellKey: key}))
		if err != nil {
			return nil, err
		}
		return res.Msg.GetSpell(), nil
	}

	d, err := get(player, campaign, "spell:fireball")
	if err != nil {
		t.Fatalf("GetSpellDetails(fireball) error = %v", err)
	}
	if d.GetSpell().GetNamePt() != "Bola de Fogo" || d.GetSpell().GetLevel() != 3 ||
		d.GetCastingTime().GetUnit() != rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_ACTION || d.GetCastingTime().GetAmount() != 1 ||
		d.GetRange().GetKind() != rulesv1.SpellRangeKind_SPELL_RANGE_KIND_RANGED || d.GetRange().GetDistanceFt() != 150 ||
		!d.GetComponents().GetVerbal() || !d.GetComponents().GetSomatic() || !d.GetComponents().GetMaterial() || d.GetComponents().GetMaterialText() == "" ||
		d.GetDuration().GetKind() != rulesv1.SpellDurationKind_SPELL_DURATION_KIND_INSTANTANEOUS ||
		d.GetSave().GetAbility() != rulesv1.Ability_ABILITY_DEXTERITY || d.GetSave().GetOnSuccess() != rulesv1.SpellSaveSuccess_SPELL_SAVE_SUCCESS_HALF ||
		len(d.GetDamage()) != 1 || d.GetDamage()[0].GetBySlotLevel()[3] != "8d6" || d.GetDamage()[0].GetDamageTypePt() == "" ||
		len(d.GetDescription()) == 0 || len(d.GetHigherLevel()) == 0 {
		t.Errorf("GetSpellDetails(fireball) = %v", d)
	}

	// A cantrip's damage grows with the character's level; healing by slot.
	if fb, _ := get(master, campaign, "spell:fire-bolt"); fb.GetDamage()[0].GetByCharacterLevel()[5] != "2d10" ||
		fb.GetAttackType() != rulesv1.SpellAttackType_SPELL_ATTACK_TYPE_RANGED {
		t.Errorf("GetSpellDetails(fire bolt) = %v", fb)
	}
	if cw, _ := get(master, campaign, "spell:cure-wounds"); cw.GetHealBySlotLevel()[2] != "2d8 + MOD" || cw.GetSave() != nil {
		t.Errorf("GetSpellDetails(cure wounds) = %v", cw)
	}

	// The spells that read hit points carry their rule (Etapa 8): Sono's pool at its own circle and
	// what each circle adds, Palavra de Poder's limit, Cura Completa's amount; any other spell has none.
	sleep, _ := get(player, campaign, "spell:sleep")
	if e := sleep.GetHitPointEffect(); e.GetKind() != rulesv1.SpellHitPointEffectKind_SPELL_HIT_POINT_EFFECT_KIND_POOL ||
		e.GetPoolDiceCount() != 5 || e.GetPoolDiceSides() != 8 || e.GetPoolDicePerLevel() != 2 || e.GetConditionKey() != "condition:unconscious" {
		t.Errorf("GetSpellDetails(sleep).hit_point_effect = %v", e)
	}
	if e := mustGet(t, get, player, campaign, "spell:power-word-kill").GetHitPointEffect(); e.GetThreshold() != 100 || !e.GetDies() {
		t.Errorf("GetSpellDetails(power word kill).hit_point_effect = %v", e)
	}
	if e := mustGet(t, get, player, campaign, "spell:heal").GetHitPointEffect(); e.GetHealAmount() != 70 || e.GetHealPerLevel() != 10 {
		t.Errorf("GetSpellDetails(heal).hit_point_effect = %v", e)
	}
	if fb, _ := get(master, campaign, "spell:fire-bolt"); fb.GetHitPointEffect() != nil {
		t.Errorf("fire bolt has a hit point effect: %v", fb.GetHitPointEffect())
	}

	for name, call := range map[string]func() error{
		"an unknown spell":    func() error { _, err := get(player, campaign, "spell:nope"); return err },
		"an empty key":        func() error { _, err := get(player, campaign, ""); return err },
		"a non-member":        func() error { _, err := get(outsider, campaign, "spell:fireball"); return err },
		"an unknown campaign": func() error { _, err := get(player, "not-a-uuid", "spell:fireball"); return err },
	} {
		wantCode(t, name, call(), connect.CodeNotFound)
	}
	_, err = h.anonymous().content.GetSpellDetails(t.Context(), connect.NewRequest(&rulesv1.GetSpellDetailsRequest{CampaignId: campaign, SpellKey: "spell:fireball"}))
	wantCode(t, "no session", err, connect.CodeUnauthenticated)
}

func mustGet(t *testing.T, get func(*user, string, string) (*rulesv1.SpellDetails, error), u *user, campaign, key string) *rulesv1.SpellDetails {
	t.Helper()
	d, err := get(u, campaign, key)
	if err != nil {
		t.Fatalf("GetSpellDetails(%s) error = %v", key, err)
	}
	return d
}

// RN-30: a campaign holds a limited number of characters and NPCs, of every
// kind, living or dead. A refusal makes nothing, a replay of an NPC already
// made is answered in a full campaign, and another campaign is not counted.
func TestRN30_TheCharacterCapPerCampaign(t *testing.T) {
	t.Parallel()
	const capacity = 3
	h := newHarnessWith(t, func(c *Config) { c.MaxCharactersPerCampaign = capacity })
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	other := h.newCampaign(master, "Outra")

	dead := player.createPensantus(t, campaign)
	master.markDead(t, dead) // a dead character still counts
	master.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_STORY, "Taverneiro", basicSheet())
	const replayKey = "7d1c2a3e-4b5f-4a6b-8c7d-9e0f1a2b3c4d"
	first, err := npcFromCreature(t, master, campaign, "monster:ogre", "Grak", charactersv1.CharacterKind_CHARACTER_KIND_MINION, replayKey) // the 3rd, within the cap
	if err != nil {
		t.Fatalf("the NPC at the cap: %v", err)
	}

	_, err = master.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_STORY, Name: "Um a mais", Sheet: basicSheet(),
	}))
	wantCode(t, "CreateCharacter() over the cap", err, connect.CodeResourceExhausted)
	_, err = player.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Outra", Sheet: pensantusSheet(),
	}))
	wantCode(t, "a player's CreateCharacter() over the cap", err, connect.CodeResourceExhausted)
	_, err = npcFromCreature(t, master, campaign, "monster:wolf", "Lobo", charactersv1.CharacterKind_CHARACTER_KIND_MINION, "0c9b8a7d-6e5f-4d3c-a2b1-0f9e8d7c6b5a")
	wantCode(t, "CreateNpcFromCreature() over the cap", err, connect.CodeResourceExhausted)

	// The same key and request still answers the NPC it made.
	again, err := npcFromCreature(t, master, campaign, "monster:ogre", "Grak", charactersv1.CharacterKind_CHARACTER_KIND_MINION, replayKey)
	if err != nil || again.GetId() != first.GetId() {
		t.Fatalf("replay in a full campaign = %v, %v; want the first NPC %s", again.GetId(), err, first.GetId())
	}
	var n int
	if err := h.pool.QueryRow(t.Context(), "SELECT count(*) FROM characters WHERE campaign_id = $1", campaign).Scan(&n); err != nil || n != capacity {
		t.Fatalf("the campaign holds %d characters, %v; want %d", n, err, capacity)
	}

	// The cap is per campaign.
	master.create(t, other, charactersv1.CharacterKind_CHARACTER_KIND_STORY, "Taverneiro", basicSheet())
}

// RN-30: two creates racing for the last place do not both pass, through
// either create.
func TestRN30_TheLastCharacterPlaceGoesToOneCreate(t *testing.T) {
	t.Parallel()
	dbtest.PoolSize(t, 14) // the racers must overlap: one connection would run them one by one
	h := newHarnessWith(t, func(c *Config) { c.MaxCharactersPerCampaign = 3 })
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master, "Mirathel")
	for _, name := range []string{"Taverneiro", "Ferreiro"} {
		master.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_STORY, name, basicSheet())
	}

	const racers = 6
	errs := make([]error, racers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range racers {
		wg.Go(func() {
			<-start
			if i%2 == 0 {
				_, errs[i] = master.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
					CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_STORY, Name: fmt.Sprintf("Corredor %d", i), Sheet: basicSheet(),
				}))
				return
			}
			_, errs[i] = npcFromCreature(t, master, campaign, "monster:wolf", fmt.Sprintf("Lobo %d", i), charactersv1.CharacterKind_CHARACTER_KIND_MINION, uuid.New().String())
		})
	}
	close(start)
	wg.Wait()

	created := 0
	for i, err := range errs {
		switch {
		case err == nil:
			created++
		case connect.CodeOf(err) != connect.CodeResourceExhausted:
			t.Errorf("racer %d: error = %v", i, err)
		}
	}
	var n int
	if err := h.pool.QueryRow(t.Context(), "SELECT count(*) FROM characters WHERE campaign_id = $1", campaign).Scan(&n); err != nil || created != 1 || n != 3 {
		t.Fatalf("%d of %d creates succeeded and the campaign holds %d characters, %v; want 1 and 3", created, racers, n, err)
	}
}

// RN-30: the server's own cap is 1,000, the 1,000th place is the last.
func TestRN30_TheDefaultCharacterCapIsAThousand(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master, "Mirathel")
	if _, err := h.pool.Exec(t.Context(), `
INSERT INTO characters (campaign_id, kind, master_user_id, status, name, sheet, story, created_at, updated_at)
SELECT $1, 'story', $2, 'active', 'Figurante ' || n::STRING, '{}', '{}', now(), now()
FROM generate_series(1, $3) AS n`, campaign, master.id, DefaultMaxCharactersPerCampaign-1); err != nil {
		t.Fatalf("seed the campaign: %v", err)
	}

	master.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_STORY, "O milésimo", basicSheet())
	_, err := master.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{
		CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_STORY, Name: "O 1001º", Sheet: basicSheet(),
	}))
	wantCode(t, "the 1,001st character", err, connect.CodeResourceExhausted)
}

// RN-30: the NPC a combat makes for a monster counts too, and a monster
// already made is found in a full campaign.
func TestRN30_TheMonsterNpcOfACombatCountsInTheCap(t *testing.T) {
	t.Parallel()
	h := newHarnessWith(t, func(c *Config) { c.MaxCharactersPerCampaign = 1 })
	master := h.newUser("Mestre")
	campaign := h.newCampaign(master, "Mirathel")
	monsterNpc := func(key string) error {
		return db.InTx(t.Context(), h.pool, func(tx pgx.Tx) error {
			_, _, err := h.svc.MonsterNpc(t.Context(), tx, campaign, master.id, key, h.clock.Now())
			return err
		})
	}
	if err := monsterNpc("monster:ogre"); err != nil {
		t.Fatalf("the monster at the cap: %v", err)
	}
	wantCode(t, "a second monster", monsterNpc("monster:wolf"), connect.CodeResourceExhausted)
	if err := monsterNpc("monster:ogre"); err != nil {
		t.Fatalf("the monster already made, in a full campaign: %v", err)
	}
}

// hillDwarfSheet is a Hill Dwarf fighter of the given level, who takes the
// average at every level: the sheet whose maximum depends on Dwarven Toughness.
func hillDwarfSheet(level int32) *charactersv1.CharacterSheet {
	return &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 15, Dexterity: 12, Constitution: 12, Intelligence: 10, Wisdom: 10, Charisma: 8},
		RaceKey:    "race:dwarf",
		SubraceKey: "subrace:hill-dwarf",
		Classes:    []*charactersv1.ClassLevel{{ClassKey: "class:fighter", Level: level}},
	}}}
}

// preview calls PreviewCharacter as u.
func (u *user) preview(t *testing.T, campaignID, characterID string, kind charactersv1.CharacterKind, sheet *charactersv1.CharacterSheet) (*charactersv1.PreviewCharacterResponse, error) {
	t.Helper()
	res, err := u.api.PreviewCharacter(t.Context(), connect.NewRequest(&charactersv1.PreviewCharacterRequest{
		CampaignId: campaignID, CharacterId: characterID, Kind: kind, Sheet: sheet,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// TestPreviewCharacterDerivesWhatTheSaveStores: the preview of a Hill Dwarf adds
// Dwarven Toughness (one hit point per level) to the dice and the Constitution
// modifier, and is the maximum that CreateCharacter then stores and GetCharacter
// derives.
func TestPreviewCharacterDerivesWhatTheSaveStores(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana, bia := h.newUser("Samuel"), h.newUser("Ana"), h.newUser("Bia")
	campaign := h.newCampaign(master, "Mirathel", ana, bia)
	player := charactersv1.CharacterKind_CHARACTER_KIND_PLAYER
	// Fighter d10, Constitution 12 + 2: modifier 2; the average of a later level is 6.
	for _, tt := range []struct {
		who          *user
		level        int32
		wantMax      int32
		wantEffects  int32
		wantFromDice int32
	}{
		{ana, 1, 12 + 1, 1, 12},
		{bia, 3, 12 + 2*8 + 3, 3, 28},
	} {
		got, err := tt.who.preview(t, campaign, "", player, hillDwarfSheet(tt.level))
		if err != nil {
			t.Fatalf("PreviewCharacter(level %d) error = %v", tt.level, err)
		}
		d := got.GetDerived()
		if d.GetHitPointsMax() != tt.wantMax || d.GetHitPointsFromEffects() != tt.wantEffects || d.GetHitPointsMax()-d.GetHitPointsFromEffects() != tt.wantFromDice {
			t.Errorf("level %d: preview max = %d from effects = %d, want %d and %d", tt.level, d.GetHitPointsMax(), d.GetHitPointsFromEffects(), tt.wantMax, tt.wantEffects)
		}
		created := tt.who.create(t, campaign, player, "Gimli", hillDwarfSheet(tt.level))
		if stored := tt.who.get(t, campaign, created.GetId()).GetDerived(); !proto.Equal(stored, d) {
			t.Errorf("level %d: preview = %v, the saved sheet derives %v", tt.level, d, stored)
		}
	}
}

// TestPreviewCharacterCountsTheTablesHitPointEffect: a table race whose trait
// changes the maximum hit points counts in the preview, negative or not.
func TestPreviewCharacterCountsTheTablesHitPointEffect(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana := h.newUser("Samuel"), h.newUser("Ana")
	campaign := h.newCampaign(master, "Mirathel", ana)
	race := testRace("Anão do Mar")
	race.Traits = []*rulesv1.TableFeature{{NamePt: "Vigor frágil", Effects: []*rulesv1.TableEffect{{Type: "modifier", Target: "hp.max", Mode: "add", Value: "-2"}}}}
	entry := master.addEntry(t, campaign, race)
	sheet := hillDwarfSheet(2)
	sheet.GetFull().RaceKey, sheet.GetFull().SubraceKey = entry.GetKey(), ""

	got, err := ana.preview(t, campaign, "", charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, sheet)
	if err != nil {
		t.Fatalf("PreviewCharacter() error = %v", err)
	}
	if got.GetDerived().GetHitPointsFromEffects() != -2 {
		t.Errorf("hit points from effects = %d, want -2", got.GetDerived().GetHitPointsFromEffects())
	}
	created := ana.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Gimli", sheet)
	if want := created.GetDerived().GetHitPointsMax(); got.GetDerived().GetHitPointsMax() != want {
		t.Errorf("preview max = %d, the saved sheet has %d", got.GetDerived().GetHitPointsMax(), want)
	}
}

// TestPreviewCharacterWritesNothing: the character's row, revision and
// campaign are the same after a preview of an edit.
func TestPreviewCharacterWritesNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana := h.newUser("Samuel"), h.newUser("Ana")
	campaign := h.newCampaign(master, "Mirathel", ana)
	c := ana.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, "Gimli", hillDwarfSheet(1))
	before := ana.get(t, campaign, c.GetId())
	count := len(ana.list(t, campaign))

	got, err := ana.preview(t, campaign, c.GetId(), 0, hillDwarfSheet(4))
	if err != nil {
		t.Fatalf("PreviewCharacter() error = %v", err)
	}
	if got.GetDerived().GetTotalLevel() != 4 {
		t.Errorf("preview total level = %d, want the draft's 4", got.GetDerived().GetTotalLevel())
	}
	if _, err := ana.preview(t, campaign, "", charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, hillDwarfSheet(2)); err != nil {
		t.Fatalf("PreviewCharacter() of a new sheet error = %v", err)
	}
	after := ana.get(t, campaign, c.GetId())
	if !proto.Equal(before, after) || after.GetRevision() != 1 {
		t.Errorf("the character changed: before %v, after %v", before, after)
	}
	if n := len(ana.list(t, campaign)); n != count {
		t.Errorf("characters = %d after a preview, want %d", n, count)
	}
}

// TestPreviewCharacterRefusesWhatTheSaveRefuses: each caller and sheet that
// CreateCharacter or UpdateCharacter refuses gets the same answer from the
// preview, and a character the caller cannot see is not_found.
func TestPreviewCharacterRefusesWhatTheSaveRefuses(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana, bia, pending := h.newUser("Samuel"), h.newUser("Ana"), h.newUser("Bia"), h.newUser("Pen")
	campaign := h.newCampaign(master, "Mirathel", ana, bia)
	h.joinPending(master, campaign, pending)
	player, enemy := charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, charactersv1.CharacterKind_CHARACTER_KIND_ENEMY
	anas := ana.create(t, campaign, player, "Gimli", hillDwarfSheet(1))
	npc := master.create(t, campaign, enemy, "Orc", enemySheet())

	create := func(u *user, kind charactersv1.CharacterKind, sheet *charactersv1.CharacterSheet) error {
		_, err := u.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{CampaignId: campaign, Kind: kind, Name: "Teste", Sheet: sheet}))
		return err
	}
	// reasonOf is the CharacterBlocked reason of a failed_precondition, or 0.
	reasonOf := func(err error) charactersv1.CharacterBlockedReason {
		if connect.CodeOf(err) != connect.CodeFailedPrecondition {
			return 0
		}
		return blocked(t, "call", err).GetReason()
	}
	same := func(name string, preview, save error) {
		t.Helper()
		if preview == nil || save == nil {
			t.Errorf("%s: preview error = %v, save error = %v, want both refused", name, preview, save)
			return
		}
		if connect.CodeOf(preview) != connect.CodeOf(save) || reasonOf(preview) != reasonOf(save) {
			t.Errorf("%s: preview answers %v, the save answers %v", name, preview, save)
		}
	}
	previewNew := func(u *user, kind charactersv1.CharacterKind, sheet *charactersv1.CharacterSheet) error {
		_, err := u.preview(t, campaign, "", kind, sheet)
		return err
	}

	same("the master with a PLAYER sheet", previewNew(master, player, hillDwarfSheet(1)), create(master, player, hillDwarfSheet(1)))
	same("a player with an NPC kind", previewNew(bia, enemy, enemySheet()), create(bia, enemy, enemySheet()))
	same("a player with a basic sheet for a player kind", previewNew(bia, player, basicSheet()), create(bia, player, basicSheet()))
	same("an unknown kind", previewNew(bia, 0, hillDwarfSheet(1)), create(bia, 0, hillDwarfSheet(1)))
	bad := hillDwarfSheet(1)
	bad.GetFull().RaceKey = "race:fantasma"
	same("an unknown race", previewNew(bia, player, bad), create(bia, player, bad))
	stranger := h.newUser("Intruso")
	same("a stranger", previewNew(stranger, player, hillDwarfSheet(1)), create(stranger, player, hillDwarfSheet(1)))
	if _, err := pending.preview(t, campaign, "", player, hillDwarfSheet(1)); err != nil {
		t.Errorf("a pending member's preview error = %v, want it accepted like their create", err)
	}

	// A character the caller cannot see.
	_, err := bia.preview(t, campaign, anas.GetId(), 0, hillDwarfSheet(2))
	wantCode(t, "another player's character", err, connect.CodeNotFound)
	_, err = bia.preview(t, campaign, npc.GetId(), 0, enemySheet())
	wantCode(t, "an NPC", err, connect.CodeNotFound)
	_, err = bia.preview(t, campaign, "not-a-uuid", 0, hillDwarfSheet(2))
	wantCode(t, "an ID that is not a UUID", err, connect.CodeNotFound)
	if _, err := master.preview(t, campaign, anas.GetId(), 0, hillDwarfSheet(2)); err != nil {
		t.Errorf("the master previewing a player's character error = %v", err)
	}
	// A sheet of the wrong shape for the character's kind.
	_, err = ana.preview(t, campaign, anas.GetId(), 0, basicSheet())
	_, saveErr := ana.update(t, anas, "Gimli", basicSheet())
	same("a basic sheet for a player's character", err, saveErr)

	// The switches and the archive: a choice the table switched off is refused for a player
	// who adds it, never for the master, and never for one the sheet already has.
	archived := master.addEntry(t, campaign, testRace("Anão Antigo"))
	if _, err := master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: archived.GetKey()})); err != nil {
		t.Fatalf("ArchiveTableEntry() error = %v", err)
	}
	old := hillDwarfSheet(1)
	old.GetFull().RaceKey, old.GetFull().SubraceKey = archived.GetKey(), ""
	if blocked := reasonOf(previewNew(bia, player, old)); blocked != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_ARCHIVED_CONTENT {
		t.Errorf("a new archived choice: reason = %v, want ARCHIVED_CONTENT", blocked)
	}
	same("a new archived choice", previewNew(bia, player, old), create(bia, player, old))
	_, err = ana.preview(t, campaign, anas.GetId(), 0, old)
	_, saveErr = ana.update(t, anas, "Gimli", old)
	same("a new archived choice on an edit", err, saveErr)

	master.setOff(t, campaign, true, "race:dwarf")
	same("a switched-off choice for a player", previewNew(bia, player, hillDwarfSheet(1)), create(bia, player, hillDwarfSheet(1)))
	if reasonOf(previewNew(bia, player, hillDwarfSheet(1))) != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_SWITCHED_OFF_CONTENT {
		t.Error("a switched-off choice for a player is not SWITCHED_OFF_CONTENT")
	}
	if _, err := ana.preview(t, campaign, anas.GetId(), 0, hillDwarfSheet(2)); err != nil {
		t.Errorf("an edit that keeps the choice the sheet already has error = %v, want it accepted like the save", err)
	}
	if _, err := master.preview(t, campaign, "", enemy, func() *charactersv1.CharacterSheet {
		s := enemySheet()
		s.GetFull().RaceKey, s.GetFull().SubraceKey = "race:dwarf", ""
		return s
	}()); err != nil {
		t.Errorf("the master with a switched-off choice error = %v, want it accepted", err)
	}
	master.setOff(t, campaign, false, "race:dwarf")

	// RN-01: once a game session starts, only the master edits the sheet.
	h.lockSheets(campaign)
	_, err = ana.preview(t, campaign, anas.GetId(), 0, hillDwarfSheet(2))
	_, saveErr = ana.update(t, ana.get(t, campaign, anas.GetId()), "Gimli", hillDwarfSheet(2))
	same("a locked character", err, saveErr)
	if got := blocked(t, "PreviewCharacter", err).GetReason(); got != charactersv1.CharacterBlockedReason_CHARACTER_BLOCKED_REASON_SHEET_LOCKED {
		t.Errorf("a locked character: reason = %v, want SHEET_LOCKED", got)
	}
	if _, err := master.preview(t, campaign, anas.GetId(), 0, hillDwarfSheet(2)); err != nil {
		t.Errorf("the master previewing a locked character error = %v", err)
	}
}

// TestListCharactersTellsTheMasterWhichSheetsHaveChoicesOpen: starting a session locks
// every living player's sheet as it is, so the master's list says which of them still
// lack skills or spells. A player never gets it, and a locked sheet has none.
func TestListCharactersTellsTheMasterWhichSheetsHaveChoicesOpen(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana, bia := h.newUser("Samuel"), h.newUser("Ana"), h.newUser("Bia")
	campaign := h.newCampaign(master, "Mirathel", ana, bia)
	player := charactersv1.CharacterKind_CHARACTER_KIND_PLAYER
	complete := ana.create(t, campaign, player, "Pensantus", pensantusSheet())
	unfinished := proto.Clone(pensantusSheet()).(*charactersv1.CharacterSheet)
	unfinished.GetFull().PreparedSpellKeys = unfinished.GetFull().PreparedSpellKeys[:4]
	partial := bia.create(t, campaign, player, "Ilaria", unfinished)

	openOf := func(who *user, id string) []*charactersv1.OpenChoice {
		for _, s := range who.list(t, campaign) {
			if s.GetId() == id {
				return s.GetOpenChoices()
			}
		}
		t.Fatalf("%s does not list %s", who.id, id)
		return nil
	}
	if got := openOf(master, complete.GetId()); len(got) != 0 {
		t.Errorf("a complete sheet has open choices %v", got)
	}
	got := openOf(master, partial.GetId())
	if len(got) != 1 || got[0].GetKind() != charactersv1.OpenChoiceKind_OPEN_CHOICE_KIND_SPELLS_PREPARED || got[0].GetMissing() != 3 {
		t.Errorf("master sees open choices %v, want 3 prepared spells missing", got)
	}
	// RN-10 and the master's say alone: the player does not get the list of what is missing.
	if got := openOf(bia, partial.GetId()); len(got) != 0 {
		t.Errorf("the player sees open choices %v", got)
	}
	// A locked sheet is no longer one that starting would lock.
	h.lockSheets(campaign)
	if got := openOf(master, partial.GetId()); len(got) != 0 {
		t.Errorf("a locked sheet has open choices %v", got)
	}
}
