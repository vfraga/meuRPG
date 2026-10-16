package characters

import (
	"context"
	"strings"
	"testing"
	"uuid"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
)

// The characterization of ListCharacters and of the Character a member reads
// (characterToProto): a golden of what the master, two players and a pending member read
// of a campaign with every kind of character in it, and the failures of the server.

// editorWorld is a campaign with a character of each kind and state: Dona's fighter with
// a choice the master left open, Caio's dead wizard, Pia's pending character with changes
// asked, a reserved character with a claim link, an NPC with a portrait and a boss.
type editorWorld struct {
	*editorTable
	ids *strings.Replacer
	all []string
}

func newEditorWorld(t *testing.T) *editorWorld {
	t.Helper()
	et := newEditorTable(t, nil)
	image := uuid.New().String()
	et.h.svc.SetGallery(&editorGallery{images: map[string][]string{et.campaign: {image}}})
	fighter := et.editorStandardFighter(t)
	open, err := editorUpdate(et.master, func() *charactersv1.UpdateCharacterRequest {
		r := editorSaveOf(fighter)
		r.Sheet.GetFull().FeatureChoiceKeys = nil
		return r
	}())
	if err != nil {
		t.Fatalf("the master takes the style out: %v", err)
	}
	wizard := et.caio.createPensantus(t, et.campaign)
	wizard = et.master.markDead(t, wizard)
	pending := et.pia.createPensantus(t, et.campaign)
	if _, err := et.master.ask(pending, "Escolha outra escola.", uuid.New().String()); err != nil {
		t.Fatalf("RequestCharacterChanges() error = %v", err)
	}
	reserved := et.master.reservedByMaster(t, et.campaign, "Reserva")
	et.master.link(t, reserved, 7)
	goblin := et.master.create(t, et.campaign, editorMinion, "Goblin", withPortrait(basicSheet(), image))
	boss := et.master.create(t, et.campaign, charactersv1.CharacterKind_CHARACTER_KIND_BOSS, "Chefe", enemySheet())
	w := &editorWorld{editorTable: et, all: []string{open.GetId(), wizard.GetId(), pending.GetId(), reserved.GetId(), goblin.GetId(), boss.GetId()}}
	w.ids = strings.NewReplacer(
		open.GetId(), "<bruna>", wizard.GetId(), "<pensantus-caio>", pending.GetId(), "<pensantus-pia>",
		reserved.GetId(), "<reserva>", goblin.GetId(), "<goblin>", boss.GetId(), "<chefe>", image, "<image>",
		et.master.id, "<mestre>", et.dona.id, "<dona>", et.caio.id, "<caio>", et.pia.id, "<pia>", et.campaign, "<campaign>",
	)
	return w
}

// editorList is what u lists, one line per character.
func (w *editorWorld) editorList(t *testing.T, u *user) string {
	t.Helper()
	var lines []string
	for _, s := range u.list(t, w.campaign) {
		lines = append(lines, w.ids.Replace(editorMessage(s.ProtoReflect())))
	}
	return strings.Join(lines, "\n")
}

// editorRead is what u reads of each character of the campaign, one line each, without
// the sheet, the story and the derived numbers (other tests freeze those); a character
// u may not see reads as its refusal.
func (w *editorWorld) editorRead(t *testing.T, u *user) string {
	t.Helper()
	var lines []string
	for _, id := range w.all {
		res, err := u.api.GetCharacter(context.Background(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: id}))
		if err != nil {
			lines = append(lines, w.ids.Replace(id+": "+editorRefusal(err)))
			continue
		}
		c := proto.CloneOf(res.Msg.GetCharacter())
		derived := c.GetDerived() != nil
		c.Sheet, c.Story, c.Derived = nil, nil, nil
		line := editorMessage(c.ProtoReflect())
		if derived {
			line += " +derived"
		}
		lines = append(lines, w.ids.Replace(line))
	}
	return strings.Join(lines, "\n")
}

func TestListCharactersEditorGoldenOfEveryViewer(t *testing.T) {
	t.Parallel()
	w := newEditorWorld(t)
	for _, v := range []struct {
		name string
		as   *user
		want []string
	}{
		{"the master", w.master, editorListMaster},
		{"Dona", w.dona, editorListDona},
		{"Caio", w.caio, editorListCaio},
		{"Pia, pending", w.pia, editorListPia},
	} {
		if got, want := w.editorList(t, v.as), strings.Join(v.want, "\n"); got != want {
			t.Errorf("%s lists:\n%s\nwant:\n%s", v.name, got, want)
		}
	}
	_, err := w.visitor.api.ListCharacters(t.Context(), connect.NewRequest(&charactersv1.ListCharactersRequest{CampaignId: w.campaign}))
	wantEditorRefusal(t, "a stranger lists", err, `not_found: campaign not found`)
}

func TestGetCharacterEditorGoldenOfEveryViewer(t *testing.T) {
	t.Parallel()
	w := newEditorWorld(t)
	for _, v := range []struct {
		name string
		as   *user
		want []string
	}{
		{"the master", w.master, editorReadMaster},
		{"Dona", w.dona, editorReadDona},
		{"Caio", w.caio, editorReadCaio},
		{"Pia, pending", w.pia, editorReadPia},
	} {
		if got, want := w.editorRead(t, v.as), strings.Join(v.want, "\n"); got != want {
			t.Errorf("%s reads:\n%s\nwant:\n%s", v.name, got, want)
		}
	}
}

// TestListCharactersEditorWhenTheServerFails: a content that cannot be read, and a stored
// sheet or story that cannot be read, are the server's failures for the list and the read.
func TestListCharactersEditorWhenTheServerFails(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	pc := et.dona.createPensantus(t, et.campaign)
	et.src.failOutside.Store(true)
	_, err := et.dona.api.ListCharacters(t.Context(), connect.NewRequest(&charactersv1.ListCharactersRequest{CampaignId: et.campaign}))
	wantEditorRefusal(t, "the content fails", err, `internal: internal error`)
	et.editorLogged(t, `"msg":"characters: cannot read rules content"`)
	et.src.failOutside.Store(false)

	et.editorCorrupt(t, pc.GetId(), "story")
	_, err = et.dona.api.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: et.campaign, CharacterId: pc.GetId()}))
	wantEditorRefusal(t, "a story that cannot be read", err, `internal: this character cannot be read right now`)
	et.editorLogged(t, `"msg":"characters: cannot read a character"`, `the story of character `+pc.GetId())
	if got := len(et.dona.list(t, et.campaign)); got != 1 {
		t.Errorf("the list with a story that cannot be read has %d characters, want 1: it reads no story", got)
	}
	et.editorCorrupt(t, pc.GetId(), "sheet")
	_, err = et.dona.api.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: et.campaign, CharacterId: pc.GetId()}))
	wantEditorRefusal(t, "a sheet that cannot be read", err, `internal: this character cannot be read right now`)
	_, err = et.master.api.ListCharacters(t.Context(), connect.NewRequest(&charactersv1.ListCharactersRequest{CampaignId: et.campaign}))
	wantEditorRefusal(t, "a list with a sheet that cannot be read", err, `internal: this character cannot be read right now`)
	et.editorLogged(t, `"msg":"characters: cannot list characters"`, `the sheet of character `+pc.GetId())
}

// The goldens, recorded from the code as it was when they were written.
var (
	editorListMaster = []string{
		`CharacterSummary{id="<bruna>" kind=CHARACTER_KIND_PLAYER state=CHARACTER_STATE_DRAFT name="Bruna" player_user_id="<dona>" player_display_name="Dona" class_summary="Guerreiro 1" race_name_pt="Humano" created_at=<time> open_choices=[OpenChoice{kind=OPEN_CHOICE_KIND_SKILLS missing=2}] pending_choice_count=1}`,
		`CharacterSummary{id="<pensantus-caio>" kind=CHARACTER_KIND_PLAYER state=CHARACTER_STATE_DEAD name="Pensantus" player_user_id="<caio>" player_display_name="Caio" class_summary="Mago 3" race_name_pt="Gnomo das Rochas" created_at=<time>}`,
		`CharacterSummary{id="<pensantus-pia>" kind=CHARACTER_KIND_PLAYER state=CHARACTER_STATE_PENDING name="Pensantus" player_user_id="<pia>" player_display_name="Pia" class_summary="Mago 3" race_name_pt="Gnomo das Rochas" created_at=<time> pending_choice_count=0 review_status=REVIEW_CHANGES_REQUESTED}`,
		`CharacterSummary{id="<reserva>" kind=CHARACTER_KIND_PLAYER state=CHARACTER_STATE_DRAFT name="Reserva" class_summary="Mago 3" race_name_pt="Gnomo das Rochas" created_at=<time> reserved=true claim_state=CLAIM_STATE_SENT claim_expires_at=<time> pending_choice_count=0}`,
		`CharacterSummary{id="<goblin>" kind=CHARACTER_KIND_MINION state=CHARACTER_STATE_DRAFT name="Goblin" created_at=<time> portrait_url="/images/<image>"}`,
		`CharacterSummary{id="<chefe>" kind=CHARACTER_KIND_BOSS state=CHARACTER_STATE_DRAFT name="Chefe" class_summary="Guerreiro 2" race_name_pt="Humano" created_at=<time>}`,
	}
	editorListDona = []string{
		`CharacterSummary{id="<bruna>" kind=CHARACTER_KIND_PLAYER state=CHARACTER_STATE_DRAFT name="Bruna" player_user_id="<dona>" player_display_name="Dona" class_summary="Guerreiro 1" race_name_pt="Humano" created_at=<time> pending_choice_count=1}`,
	}
	editorListCaio = []string{
		`CharacterSummary{id="<pensantus-caio>" kind=CHARACTER_KIND_PLAYER state=CHARACTER_STATE_DEAD name="Pensantus" player_user_id="<caio>" player_display_name="Caio" class_summary="Mago 3" race_name_pt="Gnomo das Rochas" created_at=<time>}`,
	}
	editorListPia = []string{
		`CharacterSummary{id="<pensantus-pia>" kind=CHARACTER_KIND_PLAYER state=CHARACTER_STATE_PENDING name="Pensantus" player_user_id="<pia>" player_display_name="Pia" class_summary="Mago 3" race_name_pt="Gnomo das Rochas" created_at=<time> pending_choice_count=0 review_status=REVIEW_CHANGES_REQUESTED}`,
	}
	editorReadMaster = []string{
		`Character{id="<bruna>" campaign_id="<campaign>" kind=CHARACTER_KIND_PLAYER state=CHARACTER_STATE_DRAFT name="Bruna" player_user_id="<dona>" player_display_name="Dona" revision=2 created_at=<time> updated_at=<time> can_edit=true can_edit_story=true can_mark_dead=true can_access_master_notes=true can_set_story_editing=true} +derived`,
		`Character{id="<pensantus-caio>" campaign_id="<campaign>" kind=CHARACTER_KIND_PLAYER state=CHARACTER_STATE_DEAD name="Pensantus" player_user_id="<caio>" player_display_name="Caio" revision=1 died_at=<time> created_at=<time> updated_at=<time> can_edit=true can_edit_story=true can_access_master_notes=true can_set_story_editing=true can_revive=true} +derived`,
		`Character{id="<pensantus-pia>" campaign_id="<campaign>" kind=CHARACTER_KIND_PLAYER state=CHARACTER_STATE_PENDING name="Pensantus" player_user_id="<pia>" player_display_name="Pia" revision=1 created_at=<time> updated_at=<time> can_edit=true can_edit_story=true can_access_master_notes=true can_set_story_editing=true can_approve=true review=CharacterReview{status=REVIEW_CHANGES_REQUESTED reason="Escolha outra escola." requested_at=<time>} can_request_changes=true} +derived`,
		`Character{id="<reserva>" campaign_id="<campaign>" kind=CHARACTER_KIND_PLAYER state=CHARACTER_STATE_DRAFT name="Reserva" revision=1 created_at=<time> updated_at=<time> can_edit=true can_edit_story=true can_access_master_notes=true reserved=true} +derived`,
		`Character{id="<goblin>" campaign_id="<campaign>" kind=CHARACTER_KIND_MINION state=CHARACTER_STATE_DRAFT name="Goblin" revision=1 created_at=<time> updated_at=<time> can_edit=true can_edit_story=true can_access_master_notes=true}`,
		`Character{id="<chefe>" campaign_id="<campaign>" kind=CHARACTER_KIND_BOSS state=CHARACTER_STATE_DRAFT name="Chefe" revision=1 created_at=<time> updated_at=<time> can_edit=true can_edit_story=true can_access_master_notes=true} +derived`,
	}
	editorReadDona = []string{
		`Character{id="<bruna>" campaign_id="<campaign>" kind=CHARACTER_KIND_PLAYER state=CHARACTER_STATE_DRAFT name="Bruna" player_user_id="<dona>" player_display_name="Dona" revision=2 created_at=<time> updated_at=<time> can_edit=true can_edit_story=true} +derived`,
		`<pensantus-caio>: not_found: character not found`,
		`<pensantus-pia>: not_found: character not found`,
		`<reserva>: not_found: character not found`,
		`<goblin>: not_found: character not found`,
		`<chefe>: not_found: character not found`,
	}
	editorReadCaio = []string{
		`<bruna>: not_found: character not found`,
		`Character{id="<pensantus-caio>" campaign_id="<campaign>" kind=CHARACTER_KIND_PLAYER state=CHARACTER_STATE_DEAD name="Pensantus" player_user_id="<caio>" player_display_name="Caio" revision=1 died_at=<time> created_at=<time> updated_at=<time>} +derived`,
		`<pensantus-pia>: not_found: character not found`,
		`<reserva>: not_found: character not found`,
		`<goblin>: not_found: character not found`,
		`<chefe>: not_found: character not found`,
	}
	editorReadPia = []string{
		`<bruna>: not_found: character not found`,
		`<pensantus-caio>: not_found: character not found`,
		`Character{id="<pensantus-pia>" campaign_id="<campaign>" kind=CHARACTER_KIND_PLAYER state=CHARACTER_STATE_PENDING name="Pensantus" player_user_id="<pia>" player_display_name="Pia" revision=1 created_at=<time> updated_at=<time> can_edit=true can_edit_story=true review=CharacterReview{status=REVIEW_CHANGES_REQUESTED reason="Escolha outra escola." requested_at=<time>} can_resubmit=true} +derived`,
		`<reserva>: not_found: character not found`,
		`<goblin>: not_found: character not found`,
		`<chefe>: not_found: character not found`,
	}
)
