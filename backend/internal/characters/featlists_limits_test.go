package characters

import (
	"errors"
	"strconv"
	"testing"

	"connectrpc.com/connect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	systemv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/system/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

const oversizedFeatList = 100_001

// oversizedFeatSheets are full sheets whose feat lists are far over the request size limits.
func oversizedFeatSheets() map[string]*charactersv1.FullSheet {
	keys := make([]string, oversizedFeatList)
	slots := make(map[string]string, oversizedFeatList)
	for i := range keys {
		keys[i] = "feat:" + strconv.Itoa(i)
		slots[keys[i]] = "slot"
	}
	return map[string]*charactersv1.FullSheet{
		"feat_keys":  {FeatKeys: keys},
		"feat_slots": {FeatSlots: slots},
		"both":       {FeatKeys: keys, FeatSlots: slots},
		"one over":   {FeatKeys: keys[:rules.MaxListLength+1]},
	}
}

// The shape of the feat lists is checked before anything walks them: with no rules content at all, the
// refusal is the size one (a sheet that got further would fail on the missing content).
func TestCheckFullSheetRefusesOversizedFeatListsFirst(t *testing.T) {
	t.Parallel()
	for name, full := range oversizedFeatSheets() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := checkFullSheet(nil, full)
			if err == nil {
				t.Fatal("checkFullSheet() error = nil, want the size refusal")
			}
			if got := connect.CodeOf(invalidArgument(err)); got != connect.CodeInvalidArgument {
				t.Fatalf("code = %v, want invalid_argument", got)
			}
		})
	}
}

// CreateCharacter, PreviewCharacter and UpdateCharacter refuse the oversized feat lists: a body over the
// read limit of the procedure is resource_exhausted, a list one over the limit of the field invalid_argument.
func TestFeatListsOverTheLimitAreRefusedByTheRPCs(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, ana := h.newUser("Samuel"), h.newUser("Ana")
	campaign := h.newCampaign(master, "Mirathel", ana)
	player := charactersv1.CharacterKind_CHARACTER_KIND_PLAYER
	anas := ana.create(t, campaign, player, "Gimli", hillDwarfSheet(1))
	for name, full := range oversizedFeatSheets() {
		check := func(call string, err error) {
			t.Helper()
			if name == "one over" {
				wantRequestRefused(t, name+": "+call, err, "sheet.full.feat_keys", systemv1.RequestRefusedReason_REQUEST_REFUSED_REASON_LIST_TOO_LONG)
				return
			}
			wantCode(t, name+": "+call, err, connect.CodeResourceExhausted)
		}
		sheet := &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: full}}
		_, err := ana.api.CreateCharacter(t.Context(), connect.NewRequest(&charactersv1.CreateCharacterRequest{CampaignId: campaign, Kind: player, Name: "Teste", Sheet: sheet}))
		check("CreateCharacter()", err)
		_, err = ana.preview(t, campaign, "", player, sheet)
		check("PreviewCharacter() new", err)
		_, err = ana.preview(t, campaign, anas.GetId(), 0, sheet)
		check("PreviewCharacter() edit", err)
		_, err = ana.update(t, anas, "Gimli", sheet)
		check("UpdateCharacter()", err)
	}
}

// wantRequestRefused fails the test unless err is invalid_argument with the RequestRefused detail of the field and reason.
func wantRequestRefused(t *testing.T, call string, err error, field string, reason systemv1.RequestRefusedReason) {
	t.Helper()
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok || ce.Code() != connect.CodeInvalidArgument {
		t.Fatalf("%s error = %v, want invalid_argument", call, err)
	}
	for _, d := range ce.Details() {
		if v, derr := d.Value(); derr == nil {
			if r, ok := v.(*systemv1.RequestRefused); ok {
				if r.GetField() != field || r.GetReason() != reason {
					t.Errorf("%s detail = field %q reason %v, want %q %v", call, r.GetField(), r.GetReason(), field, reason)
				}
				return
			}
		}
	}
	t.Fatalf("%s: no RequestRefused detail in %v", call, err)
}
