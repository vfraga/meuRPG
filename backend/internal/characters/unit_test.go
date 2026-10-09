package characters

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// These tests need no database, so they also run in `go test ./...`
// without MEURPG_TEST_DATABASE_URL.

// lazyPool is a pool that never connects: pgxpool connects on first use,
// and these tests never get that far.
func lazyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), "postgresql://nobody@127.0.0.1:1/none")
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type noProfiles struct{}

func (noProfiles) DisplayNames(context.Context, []string) (map[string]string, error) {
	return nil, errors.New("no profiles in this test")
}

// noMembers is a MembershipSource and a PendingMembers that is never
// reached in these tests.
type noMembers struct{}

func (noMembers) CampaignMembership(context.Context, string, string) (authz.Role, authz.Status, error) {
	return "", "", authz.ErrNotMember
}

func (noMembers) ActivatePendingMember(context.Context, pgx.Tx, string, string) error {
	return errors.New("not in this test")
}

func (noMembers) DeletePendingMember(context.Context, pgx.Tx, string, string) error {
	return errors.New("not in this test")
}

func (noMembers) ClearPendingExpiry(context.Context, pgx.Tx, string, string) error {
	return errors.New("not in this test")
}

// offlineService is a Service whose database cannot be reached.
func offlineService(t *testing.T) *Service {
	t.Helper()
	svc, err := New(Config{Pool: lazyPool(t), Profiles: noProfiles{}, Members: noMembers{}, Content: NewSRDSource(loadRules(t)), SRD: loadRules(t), Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return svc
}

func TestNewValidatesItsConfig(t *testing.T) {
	t.Parallel()
	pool, content := lazyPool(t), loadRules(t)
	for name, cfg := range map[string]Config{
		"no Pool":     {Profiles: noProfiles{}, Members: noMembers{}, Content: NewSRDSource(content), SRD: content},
		"no Profiles": {Pool: pool, Members: noMembers{}, Content: NewSRDSource(content), SRD: content},
		"no Members":  {Pool: pool, Profiles: noProfiles{}, Content: NewSRDSource(content), SRD: content},
		"no Content":  {Pool: pool, Profiles: noProfiles{}, Members: noMembers{}, SRD: content},
		"no SRD":      {Pool: pool, Profiles: noProfiles{}, Members: noMembers{}, Content: NewSRDSource(content)},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("New() with %s succeeded", name)
		}
	}
}

// TestEveryMethodNeedsASession calls every method of both services signed
// out. The session check comes first, so no database is needed to see it
// refuse, and the refusal is not cacheable either.
func TestEveryMethodNeedsASession(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	offlineService(t).Mount(mux.Handle, testSessions, noMembers{}, connect.WithRequireConnectProtocolHeader())
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	c := charactersv1connect.NewCharacterServiceClient(server.Client(), server.URL)
	content := rulesv1connect.NewContentServiceClient(server.Client(), server.URL)
	table := rulesv1connect.NewTableContentServiceClient(server.Client(), server.URL)
	ctx := t.Context()
	id := "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e001"

	calls := map[string]error{}
	_, calls["CreateCharacter"] = c.CreateCharacter(ctx, connect.NewRequest(&charactersv1.CreateCharacterRequest{CampaignId: id}))
	_, calls["CreateNpcFromCreature"] = c.CreateNpcFromCreature(ctx, connect.NewRequest(&charactersv1.CreateNpcFromCreatureRequest{CampaignId: id, CreatureKey: "monster:ogre"}))
	_, calls["GetCharacter"] = c.GetCharacter(ctx, connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: id, CharacterId: id}))
	_, calls["ListCharacters"] = c.ListCharacters(ctx, connect.NewRequest(&charactersv1.ListCharactersRequest{CampaignId: id}))
	_, calls["UpdateCharacter"] = c.UpdateCharacter(ctx, connect.NewRequest(&charactersv1.UpdateCharacterRequest{CampaignId: id, CharacterId: id}))
	_, calls["UpdateCharacterStory"] = c.UpdateCharacterStory(ctx, connect.NewRequest(&charactersv1.UpdateCharacterStoryRequest{CampaignId: id, CharacterId: id}))
	_, calls["SetStoryEditing"] = c.SetStoryEditing(ctx, connect.NewRequest(&charactersv1.SetStoryEditingRequest{CampaignId: id, CharacterId: id}))
	_, calls["MarkCharacterDead"] = c.MarkCharacterDead(ctx, connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: id, CharacterId: id}))
	_, calls["GetMasterNotes"] = c.GetMasterNotes(ctx, connect.NewRequest(&charactersv1.GetMasterNotesRequest{CampaignId: id, CharacterId: id}))
	_, calls["UpdateMasterNotes"] = c.UpdateMasterNotes(ctx, connect.NewRequest(&charactersv1.UpdateMasterNotesRequest{CampaignId: id, CharacterId: id}))
	_, calls["ApproveCharacter"] = c.ApproveCharacter(ctx, connect.NewRequest(&charactersv1.ApproveCharacterRequest{CampaignId: id, CharacterId: id}))
	_, calls["RejectCharacter"] = c.RejectCharacter(ctx, connect.NewRequest(&charactersv1.RejectCharacterRequest{CampaignId: id, CharacterId: id}))
	_, calls["GetLevelUpOptions"] = c.GetLevelUpOptions(ctx, connect.NewRequest(&charactersv1.GetLevelUpOptionsRequest{CampaignId: id, CharacterId: id}))
	_, calls["PreviewLevelUp"] = c.PreviewLevelUp(ctx, connect.NewRequest(&charactersv1.PreviewLevelUpRequest{CampaignId: id, CharacterId: id}))
	_, calls["PreviewCharacter"] = c.PreviewCharacter(ctx, connect.NewRequest(&charactersv1.PreviewCharacterRequest{CampaignId: id}))
	_, calls["RollLevelUpHitPoints"] = c.RollLevelUpHitPoints(ctx, connect.NewRequest(&charactersv1.RollLevelUpHitPointsRequest{CampaignId: id, CharacterId: id}))
	_, calls["LevelUpCharacter"] = c.LevelUpCharacter(ctx, connect.NewRequest(&charactersv1.LevelUpCharacterRequest{CampaignId: id, CharacterId: id}))
	_, calls["GetAbilityRolls"] = c.GetAbilityRolls(ctx, connect.NewRequest(&charactersv1.GetAbilityRollsRequest{CampaignId: id}))
	_, calls["RollAbilityScores"] = c.RollAbilityScores(ctx, connect.NewRequest(&charactersv1.RollAbilityScoresRequest{CampaignId: id}))
	_, calls["ListLevelUps"] = c.ListLevelUps(ctx, connect.NewRequest(&charactersv1.ListLevelUpsRequest{CampaignId: id}))
	_, calls["ListCharacterCreatures"] = c.ListCharacterCreatures(ctx, connect.NewRequest(&charactersv1.ListCharacterCreaturesRequest{CampaignId: id, CharacterId: id}))
	_, calls["GiveCreature"] = c.GiveCreature(ctx, connect.NewRequest(&charactersv1.GiveCreatureRequest{CampaignId: id, CharacterId: id, MonsterKey: "monster:wolf"}))
	_, calls["RenameCreature"] = c.RenameCreature(ctx, connect.NewRequest(&charactersv1.RenameCreatureRequest{CampaignId: id, CreatureId: id, Name: "Presa"}))
	_, calls["DismissCreature"] = c.DismissCreature(ctx, connect.NewRequest(&charactersv1.DismissCreatureRequest{CampaignId: id, CreatureId: id}))
	_, calls["AdjustCreatureHitPoints"] = c.AdjustCreatureHitPoints(ctx, connect.NewRequest(&charactersv1.AdjustCreatureHitPointsRequest{CampaignId: id, CreatureId: id}))
	_, calls["GetSummonOptions"] = c.GetSummonOptions(ctx, connect.NewRequest(&charactersv1.GetSummonOptionsRequest{CampaignId: id, CharacterId: id}))
	_, calls["ListWildShapeForms"] = c.ListWildShapeForms(ctx, connect.NewRequest(&charactersv1.ListWildShapeFormsRequest{CampaignId: id, CharacterId: id}))
	_, calls["ListContent"] = content.ListContent(ctx, connect.NewRequest(&rulesv1.ListContentRequest{CampaignId: id}))
	_, calls["GetSpellDetails"] = content.GetSpellDetails(ctx, connect.NewRequest(&rulesv1.GetSpellDetailsRequest{CampaignId: id, SpellKey: "spell:fire-bolt"}))
	_, calls["ListSpells"] = content.ListSpells(ctx, connect.NewRequest(&rulesv1.ListSpellsRequest{CampaignId: id}))
	_, calls["ListCreatures"] = content.ListCreatures(ctx, connect.NewRequest(&rulesv1.ListCreaturesRequest{CampaignId: id}))
	_, calls["GetCreature"] = content.GetCreature(ctx, connect.NewRequest(&rulesv1.GetCreatureRequest{CampaignId: id, Key: "monster:wolf"}))
	_, calls["ListTrapPresets"] = content.ListTrapPresets(ctx, connect.NewRequest(&rulesv1.ListTrapPresetsRequest{CampaignId: id}))
	_, calls["ListLightPresets"] = content.ListLightPresets(ctx, connect.NewRequest(&rulesv1.ListLightPresetsRequest{CampaignId: id}))
	_, calls["ListTableEntries"] = table.ListTableEntries(ctx, connect.NewRequest(&rulesv1.ListTableEntriesRequest{CampaignId: id}))
	_, calls["GetClassTableDefaults"] = table.GetClassTableDefaults(ctx, connect.NewRequest(&rulesv1.GetClassTableDefaultsRequest{CampaignId: id}))
	_, calls["GetEffectMenu"] = table.GetEffectMenu(ctx, connect.NewRequest(&rulesv1.GetEffectMenuRequest{CampaignId: id}))
	_, calls["ListOptionSwitches"] = table.ListOptionSwitches(ctx, connect.NewRequest(&rulesv1.ListOptionSwitchesRequest{CampaignId: id}))
	_, calls["SetOptionSwitches"] = table.SetOptionSwitches(ctx, connect.NewRequest(&rulesv1.SetOptionSwitchesRequest{CampaignId: id}))
	_, calls["CreateTableEntry"] = table.CreateTableEntry(ctx, connect.NewRequest(&rulesv1.CreateTableEntryRequest{CampaignId: id}))
	_, calls["UpdateTableEntry"] = table.UpdateTableEntry(ctx, connect.NewRequest(&rulesv1.UpdateTableEntryRequest{CampaignId: id, Key: "spell:x@mesa"}))
	_, calls["ArchiveTableEntry"] = table.ArchiveTableEntry(ctx, connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: id, Key: "spell:x@mesa"}))
	_, calls["UnarchiveTableEntry"] = table.UnarchiveTableEntry(ctx, connect.NewRequest(&rulesv1.UnarchiveTableEntryRequest{CampaignId: id, Key: "spell:x@mesa"}))

	methods := charactersv1.File_meurpg_characters_v1_characters_proto.Services().ByName("CharacterService").Methods().Len() +
		rulesv1.File_meurpg_rules_v1_rules_proto.Services().ByName("ContentService").Methods().Len() +
		rulesv1.File_meurpg_rules_v1_table_content_proto.Services().ByName("TableContentService").Methods().Len()
	if len(calls) != methods {
		t.Errorf("called %d methods, the services have %d", len(calls), methods)
	}
	for name, err := range calls {
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("%s signed out: error = %v, want unauthenticated", name, err)
		}
		if ce, ok := errors.AsType[*connect.Error](err); !ok || !slices.Equal(ce.Meta().Values("Cache-Control"), []string{"no-store"}) {
			t.Errorf("%s: error response Cache-Control = %q, want no-store, once", name, ce.Meta().Values("Cache-Control"))
		}
	}
}

// TestReadsWithIDsArePostOnly: every read carries a campaign ID, so it is
// IDEMPOTENT, not NO_SIDE_EFFECTS, and the server refuses it over GET,
// which would put the ID in the URL.
func TestReadsWithIDsArePostOnly(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	offlineService(t).Mount(mux.Handle, testSessions, noMembers{}, connect.WithRequireConnectProtocolHeader())

	message := `{"campaignId":"6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e001","characterId":"6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e001"}`
	reads := map[string]protoreflect.MethodDescriptor{}
	for _, service := range []protoreflect.ServiceDescriptor{
		charactersv1.File_meurpg_characters_v1_characters_proto.Services().ByName("CharacterService"),
		rulesv1.File_meurpg_rules_v1_rules_proto.Services().ByName("ContentService"),
		rulesv1.File_meurpg_rules_v1_table_content_proto.Services().ByName("TableContentService"),
	} {
		for i := range service.Methods().Len() {
			m := service.Methods().Get(i)
			if strings.HasPrefix(string(m.Name()), "Get") || strings.HasPrefix(string(m.Name()), "List") || strings.HasPrefix(string(m.Name()), "Preview") {
				reads["/"+string(service.FullName())+"/"+string(m.Name())] = m
			}
		}
	}
	if len(reads) != 22 {
		t.Errorf("found %d reads, want 22 (GetAbilityRolls, GetCharacter, ListCharacters, GetMasterNotes, GetLevelUpOptions, PreviewLevelUp, PreviewCharacter, ListLevelUps, ListCharacterCreatures, GetSummonOptions, ListWildShapeForms, ListContent, GetSpellDetails, ListSpells, ListCreatures, GetCreature, ListTrapPresets, ListLightPresets, ListTableEntries, ListOptionSwitches, GetClassTableDefaults, GetEffectMenu)", len(reads))
	}
	for procedure, method := range reads {
		opts, _ := method.Options().(*descriptorpb.MethodOptions)
		if opts.GetIdempotencyLevel() != descriptorpb.MethodOptions_IDEMPOTENT {
			t.Errorf("%s idempotency_level = %v, want IDEMPOTENT", procedure, opts.GetIdempotencyLevel())
		}
		target := procedure + "?connect=v1&encoding=json&message=" + url.QueryEscape(message)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s: status = %d, want 405", procedure, rec.Code)
		}
	}
}

func TestCharacterState(t *testing.T) {
	t.Parallel()
	locked := time.Now()
	tests := []struct {
		status   string
		lockedAt *time.Time
		want     charactersv1.CharacterState
	}{
		{statusActive, nil, charactersv1.CharacterState_CHARACTER_STATE_DRAFT},
		{statusActive, &locked, charactersv1.CharacterState_CHARACTER_STATE_LOCKED},
		{statusDead, nil, charactersv1.CharacterState_CHARACTER_STATE_DEAD},
		{statusDead, &locked, charactersv1.CharacterState_CHARACTER_STATE_DEAD},
		{statusPending, nil, charactersv1.CharacterState_CHARACTER_STATE_PENDING},
	}
	for _, tt := range tests {
		if got := characterState(tt.status, tt.lockedAt); got != tt.want {
			t.Errorf("characterState(%q, locked %v) = %v, want %v", tt.status, tt.lockedAt != nil, got, tt.want)
		}
	}
}

// TestPermissions checks the can_* fields of Character for each caller and
// state: they tell the app which buttons to show, so they must say what the
// handlers enforce.
func TestPermissions(t *testing.T) {
	t.Parallel()
	svc := offlineService(t)
	owner, campaign := "owner-id", "campaign-id"
	master := authz.Membership{CampaignID: campaign, UserID: "master-id", Role: authz.RoleMaster}
	player := authz.Membership{CampaignID: campaign, UserID: owner, Role: authz.RolePlayer}
	locked := time.Now()
	sheet, err := storeJSON.Marshal(pensantusSheet())
	if err != nil {
		t.Fatal(err)
	}
	row := func(kind, status string, lockedAt *time.Time, storyAllowed bool) charactersdb.Character {
		r := charactersdb.Character{
			ID: "c", CampaignID: &campaign, Kind: kind, Status: status, Name: "X",
			Sheet: sheet, Story: []byte("{}"), Revision: 1, SheetLockedAt: lockedAt, StoryEditingAllowed: storyAllowed,
		}
		if kind == kindPlayer {
			r.PlayerUserID = &owner
		} else {
			r.MasterUserID = &master.UserID
		}
		if status == statusDead {
			r.DiedAt = &locked
		}
		return r
	}
	// The owner of a pending character is a pending member (RN-15).
	pendingOwner := authz.Membership{CampaignID: campaign, UserID: owner, Role: authz.RolePlayer, Pending: true}
	// want: can_edit, can_edit_story, can_mark_dead, can_access_master_notes, can_set_story_editing, can_approve
	tests := []struct {
		name string
		row  charactersdb.Character
		m    authz.Membership
		want [6]bool
	}{
		{"master, draft", row(kindPlayer, statusActive, nil, false), master, [6]bool{true, true, true, true, true, false}},
		{"master, dead", row(kindPlayer, statusDead, &locked, false), master, [6]bool{true, true, false, true, true, false}},
		{"master, NPC", row("enemy", statusActive, nil, false), master, [6]bool{true, true, false, true, false, false}},
		{"master, pending", row(kindPlayer, statusPending, nil, false), master, [6]bool{true, true, false, true, true, true}},
		{"owner, draft", row(kindPlayer, statusActive, nil, false), player, [6]bool{true, true, false, false, false, false}},
		{"owner, locked", row(kindPlayer, statusActive, &locked, false), player, [6]bool{false, false, false, false, false, false}},
		{"owner, locked, story allowed", row(kindPlayer, statusActive, &locked, true), player, [6]bool{false, true, false, false, false, false}},
		{"owner, dead", row(kindPlayer, statusDead, &locked, false), player, [6]bool{false, false, false, false, false, false}},
		{"owner, dead, story allowed", row(kindPlayer, statusDead, &locked, true), player, [6]bool{false, true, false, false, false, false}},
		{"owner, pending", row(kindPlayer, statusPending, nil, false), pendingOwner, [6]bool{true, true, false, false, false, false}},
	}
	for _, tt := range tests {
		c, err := svc.characterToProto(loadRules(t), tt.row, tt.m, "")
		if err != nil {
			t.Fatalf("%s: characterToProto() error = %v", tt.name, err)
		}
		got := [6]bool{c.GetCanEdit(), c.GetCanEditStory(), c.GetCanMarkDead(), c.GetCanAccessMasterNotes(), c.GetCanSetStoryEditing(), c.GetCanApprove()}
		if got != tt.want {
			t.Errorf("%s: can edit, edit story, mark dead, notes, set story editing, approve = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCanSee(t *testing.T) {
	t.Parallel()
	owner, other := "owner", "other"
	master := authz.Membership{UserID: "master", Role: authz.RoleMaster}
	player := authz.Membership{UserID: owner, Role: authz.RolePlayer}
	pending := authz.Membership{UserID: owner, Role: authz.RolePlayer, Pending: true}
	for _, tt := range []struct {
		name   string
		m      authz.Membership
		kind   string
		status string
		player *string
		want   bool
	}{
		{"master sees a player character", master, kindPlayer, statusActive, &owner, true},
		{"master sees a pending character", master, kindPlayer, statusPending, &owner, true},
		{"master sees an NPC", master, "boss", statusActive, nil, true},
		{"master sees an orphaned character", master, kindPlayer, statusActive, nil, true},
		{"owner sees theirs", player, kindPlayer, statusActive, &owner, true},
		{"player does not see another's", player, kindPlayer, statusActive, &other, false},
		{"player does not see one without a player", player, kindPlayer, statusActive, nil, false},
		{"player does not see an NPC", player, "story", statusActive, nil, false},
		// RN-15: a pending member sees only their own pending character.
		{"pending member sees their pending character", pending, kindPlayer, statusPending, &owner, true},
		{"pending member does not see their older active one", pending, kindPlayer, statusActive, &owner, false},
		{"pending member does not see their dead one", pending, kindPlayer, statusDead, &owner, false},
		{"pending member does not see another's pending one", pending, kindPlayer, statusPending, &other, false},
		{"pending member does not see an NPC", pending, "story", statusActive, nil, false},
	} {
		if got := canSee(tt.m, tt.kind, tt.status, tt.player); got != tt.want {
			t.Errorf("%s: canSee() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestBuildOf: the proto sheet reaches package rules as the same Build that
// the rules tests use for Pensantus, so the numbers are the same.
func TestBuildOf(t *testing.T) {
	t.Parallel()
	content := loadRules(t)
	b := buildOf(pensantusSheet().GetFull())
	if err := rules.Validate(b, content); err != nil {
		t.Fatalf("rules.Validate(Pensantus) error = %v", err)
	}
	d := rules.Derive(b, content)
	if len(d.Issues) != 0 {
		t.Errorf("Derive(Pensantus) issues = %+v, want none", d.Issues)
	}
	if b.CustomBackgroundName != "Sábio" || !slices.Equal(b.CustomBackgroundSkills, []string{"skill:arcana", "skill:history"}) {
		t.Errorf("custom background = %q %v", b.CustomBackgroundName, b.CustomBackgroundSkills)
	}
	if len(b.Classes) != 1 || b.Classes[0] != (rules.ClassLevel{Class: "class:wizard", Subclass: "subclass:evocation", Level: 3}) {
		t.Errorf("classes = %+v", b.Classes)
	}
	if b.BaseScores[rules.INT] != 16 || b.ExtraAbilityBonuses[rules.INT] != 0 || len(b.ExtraAbilityBonuses) != 6 {
		t.Errorf("scores = %v, bonuses = %v", b.BaseScores, b.ExtraAbilityBonuses)
	}

	rolled := &charactersv1.FullSheet{
		HitPoints:  &charactersv1.HitPoints{Method: charactersv1.HitPointsMethod_HIT_POINTS_METHOD_ROLLED, Rolls: []int32{4, 6}},
		Classes:    []*charactersv1.ClassLevel{{ClassKey: "class:wizard", Level: 3, Subclass: &charactersv1.ClassLevel_CustomSubclassName{CustomSubclassName: "Cronurgia"}}},
		Background: &charactersv1.FullSheet_BackgroundKey{BackgroundKey: "background:acolyte"},
	}
	b = buildOf(rolled)
	if b.HitPoints.Method != rules.HitPointsRolled || !slices.Equal(b.HitPoints.Rolls, []int{4, 6}) {
		t.Errorf("hit points = %+v, want rolled 4 and 6", b.HitPoints)
	}
	if b.Classes[0].CustomSubclassName != "Cronurgia" || b.Classes[0].Subclass != "" || b.Background != "background:acolyte" {
		t.Errorf("custom subclass or background lost: %+v", b)
	}
}

// TestDerivedToProto checks the mapping on Pensantus: the numbers of the
// ADR-0008 table come through unchanged.
func TestDerivedToProto(t *testing.T) {
	t.Parallel()
	d := derivedToProto(rules.Derive(buildOf(pensantusSheet().GetFull()), loadRules(t)))

	if d.GetContentVersion() == "" || d.GetRaceNamePt() == "" || d.GetBackgroundNamePt() != "Sábio" {
		t.Errorf("version %q, race %q, background %q", d.GetContentVersion(), d.GetRaceNamePt(), d.GetBackgroundNamePt())
	}
	if len(d.GetAbilities()) != 6 || d.GetAbilities()[3].GetAbility() != rulesv1.Ability_ABILITY_INTELLIGENCE ||
		d.GetAbilities()[3].GetScore() != 18 || d.GetAbilities()[3].GetModifier() != 4 {
		t.Errorf("abilities = %v, want INT 18 (+4) fourth", d.GetAbilities())
	}
	if d.GetTotalLevel() != 3 || d.GetProficiencyBonus() != 2 || d.GetHitPointsMax() != 23 || d.GetArmorClass() != 13 ||
		d.GetSpeedWalkFt() != 25 || d.GetPassivePerception() != 11 || d.GetInitiative() != 3 {
		t.Errorf("level %d, proficiency %d, HP %d, AC %d, speed %d, passive %d, initiative %d; want 3, 2, 23, 13, 25, 11, 3",
			d.GetTotalLevel(), d.GetProficiencyBonus(), d.GetHitPointsMax(), d.GetArmorClass(), d.GetSpeedWalkFt(), d.GetPassivePerception(), d.GetInitiative())
	}
	if sc := d.GetSpellcasting(); len(sc) != 1 || sc[0].GetSaveDc() != 14 || sc[0].GetAttackBonus() != 6 ||
		sc[0].GetCantripsKnown() != 3 || sc[0].GetPreparedMax() != 7 || sc[0].GetSpellsKnown() != 0 ||
		sc[0].GetAbility() != rulesv1.Ability_ABILITY_INTELLIGENCE {
		t.Errorf("spellcasting = %v, want INT, DC 14, +6, 3 cantrips, 7 prepared", sc)
	}
	slots := d.GetSpellSlots()
	if len(slots) != 2 || slots[0].GetLevel() != 1 || slots[0].GetCount() != 4 || slots[1].GetLevel() != 2 || slots[1].GetCount() != 2 {
		t.Errorf("spell slots = %v, want 4 x 1st and 2 x 2nd", slots)
	}
	if d.GetPactMagic() != nil {
		t.Errorf("pact magic = %v, want none", d.GetPactMagic())
	}
	if hd := d.GetHitDice(); len(hd) != 1 || hd[0].GetFaces() != 6 || hd[0].GetCount() != 3 {
		t.Errorf("hit dice = %v, want 3d6", hd)
	}
	if s := d.GetSenses(); len(s) != 1 || s[0].GetRangeFt() != 60 || !strings.HasPrefix(s[0].GetKey(), "trait:") {
		t.Errorf("senses = %v, want darkvision 60 ft keyed by the trait that grants it", s)
	}
	skills := map[string]*rulesv1.DerivedSkill{}
	for _, s := range d.GetSkills() {
		skills[s.GetKey()] = s
	}
	for key, want := range map[string]int32{"skill:arcana": 6, "skill:history": 6, "skill:investigation": 6, "skill:insight": 3} {
		if got := skills[key]; got.GetBonus() != want || got.GetProficiency() != rulesv1.ProficiencyLevel_PROFICIENCY_LEVEL_PROFICIENT {
			t.Errorf("%s = %v, want proficient %+d", key, got, want)
		}
	}
	if len(d.GetSkills()) != 18 || skills["skill:athletics"].GetProficiency() != rulesv1.ProficiencyLevel_PROFICIENCY_LEVEL_NONE {
		t.Errorf("skills = %d, athletics = %v; want 18, not proficient", len(d.GetSkills()), skills["skill:athletics"])
	}
	saves := d.GetSavingThrows()
	if len(saves) != 6 || !saves[3].GetProficient() || saves[3].GetBonus() != 6 || !saves[4].GetProficient() || saves[4].GetBonus() != 3 {
		t.Errorf("saves = %v, want INT +6 and WIS +3 proficient", saves)
	}
	var weapon, cantrip *rulesv1.Attack
	for _, a := range d.GetAttacks() {
		switch a.GetKind() {
		case rulesv1.AttackKind_ATTACK_KIND_WEAPON:
			if a.GetKey() != "attack:unarmed-strike" {
				weapon = a
			}
		case rulesv1.AttackKind_ATTACK_KIND_SPELL:
			if a.GetKey() == "spell:fire-bolt" {
				cantrip = a
			}
		}
	}
	if weapon.GetKey() != "equipment:quarterstaff" || weapon.GetVersatileDamage() == "" || weapon.GetDamageTypePt() == "" {
		t.Errorf("weapon attack = %v, want the quarterstaff, versatile, with a damage type", weapon)
	}
	if cantrip.GetAttackBonus() != 6 || cantrip.GetRangeFt() != 120 {
		t.Errorf("Fire Bolt attack = %v, want +6 at 120 ft", cantrip)
	}
	if len(d.GetSpells()) != 13 {
		t.Errorf("spells = %d, want 13 (3 cantrips and 10 in the spellbook)", len(d.GetSpells()))
	}
	for _, sp := range d.GetSpells() {
		if sp.GetSpell().GetKey() == "spell:find-familiar" && sp.GetPrepared() {
			t.Errorf("Find Familiar is prepared, but it is only in the spellbook")
		}
	}
	if len(d.GetFeatures()) == 0 || len(d.GetHints()) == 0 || len(d.GetLanguages()) == 0 || len(d.GetProficiencies().GetWeapons()) == 0 {
		t.Errorf("features %d, hints %d, languages %d, weapon proficiencies %d; want some of each",
			len(d.GetFeatures()), len(d.GetHints()), len(d.GetLanguages()), len(d.GetProficiencies().GetWeapons()))
	}
	for _, f := range d.GetFeatures() {
		if f.GetKey() == "" || f.GetNamePt() == "" || f.GetSourcePt() == "" {
			t.Errorf("feature %v lacks a key, a name or a source", f)
		}
	}
	if len(d.GetIssues()) != 0 {
		t.Errorf("issues = %v, want none", d.GetIssues())
	}
}

func TestCatalogToProto(t *testing.T) {
	t.Parallel()
	content := loadRules(t)
	catalog := content.Catalog()
	c := catalogToProto(catalog)
	if c.GetContentVersion() != content.Version() || c.GetAttribution() != catalog.Attribution || !strings.Contains(c.GetAttribution(), "SRD 5.1") {
		t.Errorf("version %q, attribution %q", c.GetContentVersion(), c.GetAttribution())
	}
	counts := map[string][2]int{
		"abilities": {len(c.GetAbilities()), len(catalog.Abilities)}, "races": {len(c.GetRaces()), len(catalog.Races)},
		"subraces": {len(c.GetSubraces()), len(catalog.Subraces)}, "classes": {len(c.GetClasses()), len(catalog.Classes)},
		"subclasses": {len(c.GetSubclasses()), len(catalog.Subclasses)}, "backgrounds": {len(c.GetBackgrounds()), len(catalog.Backgrounds)},
		"skills": {len(c.GetSkills()), len(catalog.Skills)}, "armor": {len(c.GetArmor()), len(catalog.Armor)},
		"weapons": {len(c.GetWeapons()), len(catalog.Weapons)}, "spells": {len(c.GetSpells()), len(catalog.Spells)},
	}
	for name, n := range counts {
		if n[0] != n[1] || n[0] == 0 {
			t.Errorf("%s: %d in the proto, %d in the catalog", name, n[0], n[1])
		}
	}
	classes := map[string]*rulesv1.CharacterClass{}
	for _, cl := range c.GetClasses() {
		classes[cl.GetKey()] = cl
	}
	wizard, fighter, paladin := classes["class:wizard"], classes["class:fighter"], classes["class:paladin"]
	if wizard.GetHitDie() != 6 || wizard.GetSpellcasting().GetPreparation() != rulesv1.SpellPreparation_SPELL_PREPARATION_SPELLBOOK ||
		wizard.GetSpellcasting().GetAbility() != rulesv1.Ability_ABILITY_INTELLIGENCE || len(wizard.GetSavingThrows()) != 2 ||
		wizard.GetSkillChoice().GetCount() != 2 {
		t.Errorf("wizard = %v", wizard)
	}
	if fighter.GetSpellcasting() != nil {
		t.Errorf("fighter spellcasting = %v, want none", fighter.GetSpellcasting())
	}
	if paladin.GetSpellcasting().GetFirstLevel() != 2 || paladin.GetSpellcasting().GetPreparation() != rulesv1.SpellPreparation_SPELL_PREPARATION_PREPARED {
		t.Errorf("paladin spellcasting = %v, want prepared from level 2", paladin.GetSpellcasting())
	}
	for _, a := range c.GetArmor() {
		if a.GetCategory() == rulesv1.ArmorCategory_ARMOR_CATEGORY_UNSPECIFIED {
			t.Errorf("armor %s has no category", a.GetKey())
		}
	}
	for _, w := range c.GetWeapons() {
		if w.GetKey() == "equipment:longbow" && !w.GetRanged() || w.GetKey() == "equipment:longsword" && w.GetRanged() {
			t.Errorf("weapon %v has the wrong range", w)
		}
	}
}

// TestCheckSheet: text is trimmed before it is stored, and every rule that
// is not package rules' gives invalid_argument naming the field.
func TestCheckSheet(t *testing.T) {
	t.Parallel()
	sheet := pensantusSheet()
	full := sheet.GetFull()
	full.Equipment[0].Name = "  Grimório  "
	full.GetCustomBackground().Name = " Sábio "
	full.Languages = []string{" Dracônico "}
	full.CustomFeaturesText = "\n  Pesquisador.\r\n"
	cleaned, err := checkSheet(loadRules(t), sheet)
	if err != nil {
		t.Fatalf("checkSheet() error = %v", err)
	}
	cf := cleaned.GetFull()
	if cf.GetEquipment()[0].GetName() != "Grimório" || cf.GetCustomBackground().GetName() != "Sábio" ||
		cf.GetLanguages()[0] != "Dracônico" || cf.GetCustomFeaturesText() != "Pesquisador." {
		t.Errorf("cleaned sheet = %v, want trimmed text", cf)
	}
	if cf.GetEquipment()[1].GetQuantity() != 1 {
		t.Errorf("quantity 0 became %d, want 1", cf.GetEquipment()[1].GetQuantity())
	}
	if full.GetEquipment()[0].GetName() != "  Grimório  " {
		t.Error("checkSheet() changed the request's sheet")
	}

	tests := []struct {
		name  string
		edit  func(*charactersv1.CharacterSheet)
		field string
	}{
		{"no sheet", func(s *charactersv1.CharacterSheet) { s.Content = nil }, "sheet"},
		{"ability 31", func(s *charactersv1.CharacterSheet) { s.GetFull().BaseScores.Intelligence = 31 }, "sheet.full.base_scores.intelligence"},
		{"ability 0", func(s *charactersv1.CharacterSheet) { s.GetFull().BaseScores.Strength = 0 }, "sheet.full.base_scores.strength"},
		{"no scores", func(s *charactersv1.CharacterSheet) { s.GetFull().BaseScores = nil }, "sheet.full.base_scores.strength"},
		{"unknown race", func(s *charactersv1.CharacterSheet) { s.GetFull().RaceKey = "race:vulcan" }, "sheet.full.race_key"},
		{"level 21", func(s *charactersv1.CharacterSheet) { s.GetFull().Classes[0].Level = 21 }, "sheet.full.classes[0].level"},
		{"manual bonus 11", func(s *charactersv1.CharacterSheet) {
			s.GetFull().ExtraAbilityBonuses = &rulesv1.AbilityScores{Charisma: 11}
		}, "sheet.full.extra_ability_bonuses.charisma"},
		{"cantrip that is not one", func(s *charactersv1.CharacterSheet) { s.GetFull().CantripKeys[0] = "spell:web" }, "sheet.full.cantrip_keys[0]"},
		{"empty custom background name", func(s *charactersv1.CharacterSheet) { s.GetFull().GetCustomBackground().Name = "  " }, "sheet.full.custom_background.name"},
		{"custom subclass with a line break", func(s *charactersv1.CharacterSheet) {
			s.GetFull().Classes[0].Subclass = &charactersv1.ClassLevel_CustomSubclassName{CustomSubclassName: "a\nb"}
		}, "sheet.full.classes[0].custom_subclass_name"},
		{"101 items", func(s *charactersv1.CharacterSheet) {
			s.GetFull().Equipment = slices.Repeat([]*charactersv1.Item{{Name: "Tocha"}}, 101)
		}, "sheet.full.equipment"},
		{"empty item", func(s *charactersv1.CharacterSheet) { s.GetFull().Equipment[0].Name = "" }, "sheet.full.equipment[0].name"},
		{"10,000 torches", func(s *charactersv1.CharacterSheet) { s.GetFull().Equipment[0].Quantity = 10000 }, "sheet.full.equipment[0].quantity"},
		{"negative gold", func(s *charactersv1.CharacterSheet) { s.GetFull().Coins.Gold = -1 }, "sheet.full.coins.gold"},
		{"21 languages", func(s *charactersv1.CharacterSheet) { s.GetFull().Languages = slices.Repeat([]string{"Élfico"}, 21) }, "sheet.full.languages"},
		{"long tool", func(s *charactersv1.CharacterSheet) {
			s.GetFull().ToolProficiencies = []string{strings.Repeat("x", 41)}
		}, "sheet.full.tool_proficiencies[0]"},
		{"too much XP", func(s *charactersv1.CharacterSheet) { s.GetFull().ExperiencePoints = 1_000_001 }, "sheet.full.experience_points"},
		{"unknown alignment", func(s *charactersv1.CharacterSheet) { s.GetFull().Alignment = 99 }, "sheet.full.alignment"},
		{"unknown hit point method", func(s *charactersv1.CharacterSheet) {
			s.GetFull().HitPoints = &charactersv1.HitPoints{Method: 7}
		}, "sheet.full.hit_points.method"},
		{"long features", func(s *charactersv1.CharacterSheet) {
			s.GetFull().CustomFeaturesText = strings.Repeat("x", maxCustomFeaturesLength+1)
		}, "sheet.full.custom_features_text"},
		{"basic: 0 hit points", func(s *charactersv1.CharacterSheet) { s.Content = basicSheet().Content; s.GetBasic().HitPointsMax = 0 }, "sheet.basic.hit_points_max"},
		{"basic: armor class 41", func(s *charactersv1.CharacterSheet) { s.Content = basicSheet().Content; s.GetBasic().ArmorClass = 41 }, "sheet.basic.armor_class"},
		{"basic: speed 301", func(s *charactersv1.CharacterSheet) { s.Content = basicSheet().Content; s.GetBasic().SpeedFt = 301 }, "sheet.basic.speed_ft"},
		{"basic: attack +31", func(s *charactersv1.CharacterSheet) { s.Content = basicSheet().Content; s.GetBasic().AttackBonus = 31 }, "sheet.basic.attack_bonus"},
		{"basic: initiative +21", func(s *charactersv1.CharacterSheet) {
			s.Content = basicSheet().Content
			s.GetBasic().InitiativeBonus = 21
		}, "sheet.basic.initiative_bonus"},
		{"basic: four attacks", func(s *charactersv1.CharacterSheet) {
			s.Content = basicSheet().Content
			a := s.GetBasic().Attacks[0]
			s.GetBasic().Attacks = []*charactersv1.BasicAttack{a, a, a, a}
		}, "sheet.basic.attacks"},
		{"basic: empty attack name", func(s *charactersv1.CharacterSheet) {
			s.Content = basicSheet().Content
			s.GetBasic().Attacks[0].Name = "  "
		}, "sheet.basic.attacks[0].name"},
		{"basic: long attack name", func(s *charactersv1.CharacterSheet) {
			s.Content = basicSheet().Content
			s.GetBasic().Attacks[0].Name = strings.Repeat("x", 41)
		}, "sheet.basic.attacks[0].name"},
		{"basic: attack roll +21", func(s *charactersv1.CharacterSheet) {
			s.Content = basicSheet().Content
			s.GetBasic().Attacks[0].AttackBonus = 21
		}, "sheet.basic.attacks[0].attack_bonus"},
		{"basic: 21 damage dice", func(s *charactersv1.CharacterSheet) {
			s.Content = basicSheet().Content
			s.GetBasic().Attacks[0].DamageDiceCount = 21
		}, "sheet.basic.attacks[0].damage_dice_count"},
		{"basic: d7", func(s *charactersv1.CharacterSheet) {
			s.Content = basicSheet().Content
			s.GetBasic().Attacks[0].DamageDiceSides = 7
		}, "sheet.basic.attacks[0].damage_dice_sides"},
		{"basic: damage bonus 41", func(s *charactersv1.CharacterSheet) {
			s.Content = basicSheet().Content
			s.GetBasic().Attacks[0].DamageBonus = 41
		}, "sheet.basic.attacks[0].damage_bonus"},
		{"basic: no damage type", func(s *charactersv1.CharacterSheet) {
			s.Content = basicSheet().Content
			s.GetBasic().Attacks[0].DamageType = 0
		}, "sheet.basic.attacks[0].damage_type"},
		{"basic: unknown damage type", func(s *charactersv1.CharacterSheet) {
			s.Content = basicSheet().Content
			s.GetBasic().Attacks[0].DamageType = 99
		}, "sheet.basic.attacks[0].damage_type"},
		{"basic: range 601", func(s *charactersv1.CharacterSheet) {
			s.Content = basicSheet().Content
			s.GetBasic().Attacks[0].RangeFt = 601
		}, "sheet.basic.attacks[0].range_ft"},
		{"basic: long damage", func(s *charactersv1.CharacterSheet) {
			s.Content = basicSheet().Content
			s.GetBasic().Attacks = nil // the old text only counts without attacks
			s.GetBasic().Damage = strings.Repeat("d", 41)
		}, "sheet.basic.damage"},
	}
	for _, tt := range tests {
		s := pensantusSheet()
		tt.edit(s)
		_, err := checkSheet(loadRules(t), s)
		fe, ok := errors.AsType[*fieldError](err)
		if !ok || fe.field != tt.field {
			t.Errorf("%s: checkSheet() error = %v, want one about %s", tt.name, err, tt.field)
			continue
		}
		if ce := invalidArgument(err); connect.CodeOf(ce) != connect.CodeInvalidArgument {
			t.Errorf("%s: invalidArgument() = %v, want invalid_argument", tt.name, ce)
		}
	}
}

func TestCheckSheetKind(t *testing.T) {
	t.Parallel()
	full, basic := pensantusSheet(), basicSheet()
	for _, tt := range []struct {
		kind  string
		sheet *charactersv1.CharacterSheet
		ok    bool
	}{
		{kindPlayer, full, true},
		{"enemy", full, true},
		{"boss", full, true},
		{"minion", basic, true},
		{"story", basic, true},
		{kindPlayer, basic, false},
		{"boss", basic, false},
		{"minion", full, false},
		{"story", full, false},
	} {
		if err := checkSheetKind(tt.kind, tt.sheet); (err == nil) != tt.ok {
			t.Errorf("checkSheetKind(%s, full %v) error = %v, want ok %v", tt.kind, tt.sheet.GetFull() != nil, err, tt.ok)
		}
	}
}

func TestCheckStory(t *testing.T) {
	t.Parallel()
	empty, err := checkStory(nil)
	if err != nil || !proto.Equal(empty, &charactersv1.CharacterStory{}) {
		t.Errorf("checkStory(nil) = %v, %v; want an empty story", empty, err)
	}

	story := &charactersv1.CharacterStory{
		Backstory:   "  Nasceu em Mirathel.\n\nEstudou magia.  ",
		Personality: &charactersv1.Personality{Traits: " Curioso "},
		Appearance:  &charactersv1.Appearance{Height: " 1,05 m ", Description: "Barba ruiva."},
	}
	cleaned, err := checkStory(story)
	if err != nil {
		t.Fatalf("checkStory() error = %v", err)
	}
	if cleaned.GetBackstory() != "Nasceu em Mirathel.\n\nEstudou magia." || cleaned.GetPersonality().GetTraits() != "Curioso" ||
		cleaned.GetAppearance().GetHeight() != "1,05 m" {
		t.Errorf("checkStory() = %v, want trimmed text", cleaned)
	}

	for _, tt := range []struct {
		story *charactersv1.CharacterStory
		field string
	}{
		{&charactersv1.CharacterStory{Backstory: strings.Repeat("é", maxBackstoryLength+1)}, "story.backstory"},
		{&charactersv1.CharacterStory{Allies: strings.Repeat("é", maxAlliesLength+1)}, "story.allies"},
		{&charactersv1.CharacterStory{Personality: &charactersv1.Personality{Flaws: strings.Repeat("x", maxPersonalityLength+1)}}, "story.personality.flaws"},
		{&charactersv1.CharacterStory{Appearance: &charactersv1.Appearance{Eyes: "azuis\nverdes"}}, "story.appearance.eyes"},
		{&charactersv1.CharacterStory{Appearance: &charactersv1.Appearance{Description: "a\x00b"}}, "story.appearance.description"},
	} {
		_, err := checkStory(tt.story)
		if fe, ok := errors.AsType[*fieldError](err); !ok || fe.field != tt.field {
			t.Errorf("checkStory() error = %v, want one about %s", err, tt.field)
		}
	}
}

// TestStoredDocuments: sheets are stored as protojson with the proto field
// names (the names buf breaking protects), and read back ignoring fields a
// newer version may have added.
func TestStoredDocuments(t *testing.T) {
	t.Parallel()
	doc, err := storeJSON.Marshal(pensantusSheet())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"full"`, `"base_scores"`, `"race_key"`, `"custom_background"`, `"ALIGNMENT_NEUTRAL_GOOD"`} {
		if !strings.Contains(string(doc), want) {
			t.Errorf("stored sheet %s lacks %s", doc, want)
		}
	}
	// The free-text equipment of a stored sheet is read as the inventory's lines, word
	// for word, and the equipment list is left empty.
	got, err := loadSheet("c", doc)
	want := pensantusSheet()
	lines := want.GetFull().GetEquipment()
	foldEquipment("c", want.GetFull())
	if err != nil || !proto.Equal(got, want) {
		t.Errorf("loadSheet(stored) = %v, %v; want the same sheet with the equipment as inventory", got, err)
	}
	if len(got.GetFull().GetEquipment()) != 0 || len(got.GetFull().GetInventory().GetItems()) != len(lines) || len(lines) == 0 {
		t.Fatalf("equipment %v, inventory %v: want the %d lines moved", got.GetFull().GetEquipment(), got.GetFull().GetInventory(), len(lines))
	}
	for i, line := range lines {
		if it := got.GetFull().GetInventory().GetItems()[i]; it.GetName() != line.GetName() || it.GetQuantity() != max(line.GetQuantity(), 1) || it.GetCatalogKey() != "" {
			t.Errorf("inventory line %d = %v, want the free-text %q", i, it, line.GetName())
		}
	}
	again, _ := loadSheet("c", doc)
	if !proto.Equal(got, again) {
		t.Error("reading the same stored sheet twice gave different inventory ids")
	}
	if _, err := loadSheet("c", []byte(`{"full":{"race_key":"race:gnome","from_the_future":1}}`)); err != nil {
		t.Errorf("loadSheet() with an unknown field error = %v, want it ignored", err)
	}
	_, err = loadSheet("c", []byte(`{"full": "texto do jogador"}`))
	if !errors.Is(err, errCorruptDocument) || strings.Contains(err.Error(), "texto do jogador") {
		t.Errorf("loadSheet(corrupt) error = %v, want errCorruptDocument without the document", err)
	}
}

// TestCatalogToProtoMaxSpellLevel: ListContent hands the browser the
// highest spell circle per class level, so it can filter the spell lists.
func TestCatalogToProtoMaxSpellLevel(t *testing.T) {
	c, err := rules.LoadSRD()
	if err != nil {
		t.Fatal(err)
	}
	out := catalogToProto(c.Catalog())
	want := map[string]int32{"class:bard": 1, "class:paladin": 0}
	for _, cl := range out.Classes {
		w, ok := want[cl.Key]
		if !ok {
			continue
		}
		got := cl.GetSpellcasting().GetMaxSpellLevelByLevel()
		if len(got) != 20 || got[0] != w {
			t.Errorf("%s max_spell_level_by_level = %v, want 20 entries starting with %d", cl.Key, got, w)
		}
		delete(want, cl.Key)
	}
	if len(want) > 0 {
		t.Errorf("classes missing from the content: %v", want)
	}
}

// TestDerivedToProtoResourcesAndDice: the combat data reaches the API.
func TestDerivedToProtoResourcesAndDice(t *testing.T) {
	t.Parallel()
	c := loadRules(t)
	d := derivedToProto(rules.Derive(buildOf(pensantusSheet().GetFull()), c))
	if r := d.GetResources(); len(r) != 1 || r[0].GetKey() != "arcane_recovery" || r[0].GetMax() != 1 ||
		r[0].GetRecharge() != rulesv1.Recharge_RECHARGE_LONG_REST || r[0].GetNamePt() != "Recuperação Arcana" {
		t.Errorf("resources = %v, want Arcane Recovery once a day", r)
	}
	if len(d.GetStandardActions()) != 10 || d.GetStandardActions()[2].GetNamePt() != "Disparada" ||
		d.GetStandardActions()[2].GetEconomy() != rulesv1.ActionEconomy_ACTION_ECONOMY_ACTION {
		t.Errorf("standard actions = %v", d.GetStandardActions())
	}
	for _, a := range d.GetAttacks() {
		if a.GetKey() == "spell:fire-bolt" && (a.GetDamageDice().GetCount() != 1 || a.GetDamageDice().GetSides() != 10 || a.GetVersatileDamageDice() != nil) {
			t.Errorf("fire bolt dice = %v / %v", a.GetDamageDice(), a.GetVersatileDamageDice())
		}
		if a.GetKey() == "equipment:quarterstaff" && (a.GetDamageDice().GetBonus() != 1 || a.GetVersatileDamageDice().GetSides() != 8 || a.GetDamageTypeKey() != "damage-type:bludgeoning") {
			t.Errorf("quarterstaff dice = %v / %v, %q", a.GetDamageDice(), a.GetVersatileDamageDice(), a.GetDamageTypeKey())
		}
	}

	fighter := rules.Derive(rules.Build{
		BaseScores: map[rules.Ability]int{rules.STR: 15, rules.DEX: 14, rules.CON: 13, rules.INT: 12, rules.WIS: 10, rules.CHA: 8},
		Race:       "race:human", Background: "background:acolyte",
		Classes: []rules.ClassLevel{{Class: "class:fighter", Level: 3}},
	}, c)
	f := derivedToProto(fighter)
	found := false
	for _, a := range f.GetActions() {
		if a.GetKey() == "feature:second-wind" {
			found = a.GetEconomy() == rulesv1.ActionEconomy_ACTION_ECONOMY_BONUS_ACTION && a.GetResourceKey() == "second_wind"
		}
	}
	if !found {
		t.Errorf("Second Wind missing from actions: %v", f.GetActions())
	}
}
