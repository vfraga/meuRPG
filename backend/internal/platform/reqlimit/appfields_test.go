package reqlimit

import (
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1/campaignsv1connect"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
	systemv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/system/v1"
)

// These tests hold the request size limits of the fields the app sends: each
// takes a value at its limit, refuses one more, and the largest request the
// app legitimately builds fits the read limit of its procedure.

const (
	listLimit = systemv1.RequestRefusedReason_REQUEST_REFUSED_REASON_LIST_TOO_LONG
	textLimit = systemv1.RequestRefusedReason_REQUEST_REFUSED_REASON_TEXT_TOO_LONG
	// emoji is a character of 4 bytes, the most a character takes in UTF-8 and in proto JSON.
	emoji = "\U0001F600"
)

// keys are n content keys of 64 bytes, longer than the app's.
func keys(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "feature:" + strings.Repeat("k", 48) + "-" + strconv.Itoa(1000+i)
	}
	return out
}

func ascii(n int) string { return strings.Repeat("a", n) }

func fullSheet(f *charactersv1.FullSheet) *charactersv1.CharacterSheet {
	return &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: f}}
}

func tableSpell(s *rulesv1.TableSpell) *rulesv1.CreateTableEntryRequest {
	return &rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableSpell{TableSpell: s}}
}

func tableFeat(f *rulesv1.TableFeat) *rulesv1.UpdateTableEntryRequest {
	return &rulesv1.UpdateTableEntryRequest{Body: &rulesv1.UpdateTableEntryRequest_TableFeat{TableFeat: f}}
}

func importPack(entries ...*rulesv1.TableEntry) *rulesv1.ImportTableContentRequest {
	return &rulesv1.ImportTableContentRequest{Pack: &rulesv1.TableContentPack{Entries: entries}}
}

// appField is one limit of the app: build(n) makes a request whose field has n
// items (a list or map) or n bytes (a string or bytes), and worst is the
// largest request the app builds for the procedure, if the field is not the
// one that decides it.
type appField struct {
	name      string
	procedure string
	field     string // the path the refusal names
	reason    systemv1.RequestRefusedReason
	limit     int
	build     func(n int) proto.Message
	worst     func() proto.Message
}

func appFields() []appField {
	const campaign = "00000000-0000-0000-0000-000000000000"
	sheetFields := func(name, field string, reason systemv1.RequestRefusedReason, limit int, set func(*charactersv1.FullSheet, int)) []appField {
		var out []appField
		for _, p := range []struct {
			proc  string
			build func(f *charactersv1.FullSheet) proto.Message
		}{
			{charactersv1connect.CharacterServiceCreateCharacterProcedure, func(f *charactersv1.FullSheet) proto.Message {
				return &charactersv1.CreateCharacterRequest{CampaignId: campaign, Sheet: fullSheet(f)}
			}},
			{charactersv1connect.CharacterServiceUpdateCharacterProcedure, func(f *charactersv1.FullSheet) proto.Message {
				return &charactersv1.UpdateCharacterRequest{CampaignId: campaign, Sheet: fullSheet(f)}
			}},
			{charactersv1connect.CharacterServicePreviewCharacterProcedure, func(f *charactersv1.FullSheet) proto.Message {
				return &charactersv1.PreviewCharacterRequest{CampaignId: campaign, Sheet: fullSheet(f)}
			}},
			{charactersv1connect.CharacterServicePreviewChoicesProcedure, func(f *charactersv1.FullSheet) proto.Message {
				return &charactersv1.PreviewChoicesRequest{CampaignId: campaign, Sheet: fullSheet(f)}
			}},
		} {
			out = append(out, appField{
				name: name + " of " + p.proc[strings.LastIndex(p.proc, "/")+1:], procedure: p.proc, field: "sheet.full." + field,
				reason: reason, limit: limit,
				build: func(n int) proto.Message {
					f := &charactersv1.FullSheet{}
					set(f, n)
					return p.build(f)
				},
			})
		}
		return out
	}
	var out []appField
	out = append(out, sheetFields("known_spell_keys", "known_spell_keys", listLimit, 200, func(f *charactersv1.FullSheet, n int) { f.KnownSpellKeys = keys(n) })...)
	out = append(out, sheetFields("known_issues", "known_issues", listLimit, 1000, func(f *charactersv1.FullSheet, n int) { f.KnownIssues = keys(n) })...)
	out = append(out, sheetFields("content_baselines", "content_baselines", listLimit, 1000, func(f *charactersv1.FullSheet, n int) {
		f.ContentBaselines = map[string]int32{}
		for i, k := range keys(n) {
			f.ContentBaselines[k] = int32(i)
		}
	})...)
	out = append(out, sheetFields("custom_features_text", "custom_features_text", textLimit, 20<<10, func(f *charactersv1.FullSheet, n int) { f.CustomFeaturesText = ascii(n) })...)

	levelUp := func(n int) *charactersv1.LevelUpChoices { return &charactersv1.LevelUpChoices{KnownSpellKeys: keys(n)} }
	out = append(out,
		appField{
			name: "level-up known_spell_keys", procedure: charactersv1connect.CharacterServiceLevelUpCharacterProcedure, field: "choices.known_spell_keys", reason: listLimit, limit: 200,
			build: func(n int) proto.Message {
				return &charactersv1.LevelUpCharacterRequest{CampaignId: campaign, Choices: levelUp(n)}
			},
		},
		appField{
			name: "preview level-up known_spell_keys", procedure: charactersv1connect.CharacterServicePreviewLevelUpProcedure, field: "choices.known_spell_keys", reason: listLimit, limit: 200,
			build: func(n int) proto.Message {
				return &charactersv1.PreviewLevelUpRequest{CampaignId: campaign, Choices: levelUp(n)}
			},
		},
		appField{
			name: "backstory", procedure: charactersv1connect.CharacterServiceUpdateCharacterStoryProcedure, field: "story.backstory", reason: textLimit, limit: 40 << 10,
			build: func(n int) proto.Message {
				return &charactersv1.UpdateCharacterStoryRequest{CampaignId: campaign, Story: &charactersv1.CharacterStory{Backstory: ascii(n)}}
			},
			worst: func() proto.Message {
				return &charactersv1.UpdateCharacterStoryRequest{CampaignId: campaign, Story: &charactersv1.CharacterStory{Backstory: strings.Repeat(emoji, 10000)}}
			},
		},
		appField{
			name: "master notes", procedure: charactersv1connect.CharacterServiceUpdateMasterNotesProcedure, field: "notes", reason: textLimit, limit: 80 << 10,
			build: func(n int) proto.Message {
				return &charactersv1.UpdateMasterNotesRequest{CampaignId: campaign, Notes: ascii(n)}
			},
			worst: func() proto.Message {
				return &charactersv1.UpdateMasterNotesRequest{CampaignId: campaign, Notes: strings.Repeat(emoji, 20000)}
			},
		},
		appField{
			name: "campaign document", procedure: campaignsv1connect.CampaignDocumentServiceUpdateCampaignDocumentProcedure, field: "body", reason: textLimit, limit: 400 << 10,
			build: func(n int) proto.Message {
				return &campaignsv1.UpdateCampaignDocumentRequest{CampaignId: campaign, Body: ascii(n)}
			},
			// The document is 200 KiB after the line breaks change, and the editor sends \n:
			// every byte a line break is the longest in proto JSON.
			worst: func() proto.Message {
				return &campaignsv1.UpdateCampaignDocumentRequest{CampaignId: campaign, Body: strings.Repeat("\n", 200<<10)}
			},
		},
		appField{
			name: "paint map cells", procedure: mapsv1connect.MapServicePaintMapCellsProcedure, field: "squares", reason: listLimit, limit: 400,
			build: func(n int) proto.Message {
				sq := make([]*mapsv1.MapSquare, n)
				for i := range sq {
					sq[i] = &mapsv1.MapSquare{Col: 99999, Row: 99999}
				}
				return &mapsv1.PaintMapCellsRequest{CampaignId: campaign, Squares: sq}
			},
		},
		appField{
			name: "create point hooks", procedure: mapsv1connect.MapServiceCreateMapPointProcedure, field: "hooks", reason: textLimit, limit: 16 << 10,
			build: func(n int) proto.Message { return &mapsv1.CreateMapPointRequest{CampaignId: campaign, Hooks: ascii(n)} },
			worst: func() proto.Message {
				return &mapsv1.CreateMapPointRequest{CampaignId: campaign, Description: strings.Repeat(emoji, 2000), Hooks: strings.Repeat(emoji, 4000)}
			},
		},
		appField{
			name: "update point hooks", procedure: mapsv1connect.MapServiceUpdateMapPointProcedure, field: "hooks", reason: textLimit, limit: 16 << 10,
			build: func(n int) proto.Message {
				return &mapsv1.UpdateMapPointRequest{CampaignId: campaign, Hooks: proto.String(ascii(n))}
			},
		},
		appField{
			name: "option switches", procedure: rulesv1connect.TableContentServiceSetOptionSwitchesProcedure, field: "switches", reason: listLimit, limit: 1400,
			build: func(n int) proto.Message {
				sw := make([]*rulesv1.OptionSwitch, n)
				for i, k := range keys(n) {
					sw[i] = &rulesv1.OptionSwitch{Key: k, Off: true}
				}
				return &rulesv1.SetOptionSwitchesRequest{CampaignId: campaign, Switches: sw}
			},
		},
		appField{
			name: "pack json", procedure: rulesv1connect.TableContentServiceImportTableContentProcedure, field: "pack_json", reason: textLimit, limit: 3 << 20,
			build: func(n int) proto.Message {
				return &rulesv1.ImportTableContentRequest{CampaignId: campaign, PackJson: []byte(ascii(n))}
			},
		},
		appField{
			name: "pack entries", procedure: rulesv1connect.TableContentServiceImportTableContentProcedure, field: "pack.entries", reason: listLimit, limit: 1000,
			build: func(n int) proto.Message {
				es := make([]*rulesv1.TableEntry, n)
				for i := range es {
					es[i] = &rulesv1.TableEntry{Key: "feat:x@mesa"}
				}
				return importPack(es...)
			},
		},
		appField{
			name: "class srd_spell_keys", procedure: rulesv1connect.TableContentServiceCreateTableEntryProcedure, field: "table_class.casting.srd_spell_keys", reason: listLimit, limit: 400,
			build: func(n int) proto.Message {
				return &rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableClass{TableClass: &rulesv1.TableClass{Casting: &rulesv1.TableCasting{SrdSpellKeys: keys(n)}}}}
			},
		},
		appField{
			name: "spell desc_pt paragraph", procedure: rulesv1connect.TableContentServiceCreateTableEntryProcedure, field: "table_spell.desc_pt", reason: textLimit, limit: 16 << 10,
			build: func(n int) proto.Message { return tableSpell(&rulesv1.TableSpell{DescPt: []string{ascii(n)}}) },
		},
		appField{
			name: "spell higher_level_pt paragraph", procedure: rulesv1connect.TableContentServiceCreateTableEntryProcedure, field: "table_spell.higher_level_pt", reason: textLimit, limit: 16 << 10,
			build: func(n int) proto.Message { return tableSpell(&rulesv1.TableSpell{HigherLevelPt: []string{ascii(n)}}) },
		},
		appField{
			name: "creature trait text_pt", procedure: rulesv1connect.TableContentServiceCreateTableEntryProcedure, field: "table_creature.traits.text_pt", reason: textLimit, limit: 16 << 10,
			build: func(n int) proto.Message {
				return &rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableCreature{TableCreature: &rulesv1.TableCreature{Traits: []*rulesv1.TableCreatureTrait{{TextPt: ascii(n)}}}}}
			},
		},
		appField{
			name: "background equipment_pt", procedure: rulesv1connect.TableContentServiceCreateTableEntryProcedure, field: "table_background.equipment_pt", reason: textLimit, limit: 16 << 10,
			build: func(n int) proto.Message {
				return &rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableBackground{TableBackground: &rulesv1.TableBackground{EquipmentPt: ascii(n)}}}
			},
		},
		appField{
			name: "spell material_pt", procedure: rulesv1connect.TableContentServiceCreateTableEntryProcedure, field: "table_spell.components.material_pt", reason: textLimit, limit: 16 << 10,
			build: func(n int) proto.Message {
				return tableSpell(&rulesv1.TableSpell{Components: &rulesv1.TableSpellComponents{MaterialPt: ascii(n)}})
			},
		},
		appField{
			name: "feat desc_pt paragraph", procedure: rulesv1connect.TableContentServiceUpdateTableEntryProcedure, field: "table_feat.desc_pt", reason: textLimit, limit: 16 << 10,
			build: func(n int) proto.Message { return tableFeat(&rulesv1.TableFeat{DescPt: []string{ascii(n)}}) },
		},
		appField{
			name: "feat effect text_pt", procedure: rulesv1connect.TableContentServiceUpdateTableEntryProcedure, field: "table_feat.effects.text_pt", reason: textLimit, limit: 16 << 10,
			build: func(n int) proto.Message {
				return tableFeat(&rulesv1.TableFeat{Effects: []*rulesv1.TableEffect{{TextPt: ascii(n)}}})
			},
		},
	)
	return out
}

// check is Check's answer as the reason and the field, or the zero value for none.
func check(t *testing.T, msg proto.Message) (systemv1.RequestRefusedReason, string) {
	t.Helper()
	if err := Check(msg); err != nil {
		r := refusalOf(t, err)
		return r.reason, r.field
	}
	return 0, ""
}

func TestTheAppsFieldsTakeTheirLimitAndRefuseOneMore(t *testing.T) {
	for _, tc := range appFields() {
		t.Run(tc.name, func(t *testing.T) {
			if reason, field := check(t, tc.build(tc.limit)); reason != 0 {
				t.Errorf("at the limit (%d): refused %v at %q", tc.limit, reason, field)
			}
			if reason, field := check(t, tc.build(tc.limit+1)); reason != tc.reason || field != tc.field {
				t.Errorf("over the limit (%d): refused %v at %q, want %v at %q", tc.limit+1, reason, field, tc.reason, tc.field)
			}
		})
	}
}

// The largest request the app builds for a procedure is read in full: its read
// limit is at least the size of that request, as proto JSON (what the web
// sends) and as proto.
func TestTheAppsLargestRequestsFitTheirReadLimit(t *testing.T) {
	for _, tc := range appFields() {
		if tc.field == "pack_json" {
			continue // base64 in proto JSON: a pack within about 50 bytes of 3 MiB is over the backstop, see TestAPackAtTheLimitFitsTheBackstopAsProto
		}
		t.Run(tc.name, func(t *testing.T) {
			msg := tc.build(tc.limit)
			if tc.worst != nil {
				msg = tc.worst()
			}
			if reason, field := check(t, msg); reason != 0 {
				t.Fatalf("the largest legitimate request is refused: %v at %q", reason, field)
			}
			asJSON, err := protojson.Marshal(msg)
			if err != nil {
				t.Fatal(err)
			}
			if n, limit := int64(len(asJSON)), ReadLimit(tc.procedure); n > limit {
				t.Errorf("%d bytes of proto JSON, the read limit of %s is %d", n, tc.procedure, limit)
			}
		})
	}
}

func TestAPackAtTheLimitFitsTheBackstopAsProto(t *testing.T) {
	msg := &rulesv1.ImportTableContentRequest{CampaignId: "00000000-0000-0000-0000-000000000000", PackJson: []byte(ascii(3 << 20))}
	if n := proto.Size(msg); int64(n) > Backstop || ReadLimit(rulesv1connect.TableContentServiceImportTableContentProcedure) < int64(n) {
		t.Errorf("a pack of 3 MiB is %d bytes of proto, over the read limit", n)
	}
}

// A batch of 700 switches, the web's call for a whole campaign
// (web/src/app/core/content/content-client.ts:111).
func TestTheWebsLargestOptionSwitchBatchIsAccepted(t *testing.T) {
	const procedure = rulesv1connect.TableContentServiceSetOptionSwitchesProcedure
	sw := make([]*rulesv1.OptionSwitch, 700)
	for i, k := range keys(700) {
		sw[i] = &rulesv1.OptionSwitch{Key: k, Off: i%2 == 0}
	}
	msg := &rulesv1.SetOptionSwitchesRequest{CampaignId: "00000000-0000-0000-0000-000000000000", Switches: sw}
	if reason, field := check(t, msg); reason != 0 {
		t.Fatalf("700 switches refused: %v at %q", reason, field)
	}
	body, err := protojson.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(body)) > ReadLimit(procedure) {
		t.Errorf("700 switches are %d bytes of proto JSON, over the read limit %d", len(body), ReadLimit(procedure))
	}
}

// The lists of the sheet that the app fills up to 100 items are accepted at 100
// and refused at 101, with no option: they keep the default.
func TestTheListsOfTheDefaultAreAcceptedAt100(t *testing.T) {
	const campaign = "00000000-0000-0000-0000-000000000000"
	items := func(n int) []*charactersv1.Item {
		out := make([]*charactersv1.Item, n)
		for i := range out {
			out[i] = &charactersv1.Item{Name: ascii(100), Quantity: 9999}
		}
		return out
	}
	tests := []struct {
		name  string
		field string
		build func(n int) proto.Message
	}{
		{"equipment", "sheet.full.equipment", func(n int) proto.Message {
			return &charactersv1.UpdateCharacterRequest{Sheet: fullSheet(&charactersv1.FullSheet{Equipment: items(n)})}
		}},
		{"feat_keys", "sheet.full.feat_keys", func(n int) proto.Message {
			return &charactersv1.UpdateCharacterRequest{Sheet: fullSheet(&charactersv1.FullSheet{FeatKeys: keys(n)})}
		}},
		{"feature_choice_keys", "sheet.full.feature_choice_keys", func(n int) proto.Message {
			return &charactersv1.UpdateCharacterRequest{Sheet: fullSheet(&charactersv1.FullSheet{FeatureChoiceKeys: keys(n)})}
		}},
		{"prepared_spell_keys", "sheet.full.prepared_spell_keys", func(n int) proto.Message {
			return &charactersv1.UpdateCharacterRequest{Sheet: fullSheet(&charactersv1.FullSheet{PreparedSpellKeys: keys(n)})}
		}},
		{"treasure_point_ids", "treasure_point_ids", func(n int) proto.Message {
			return &progressionv1.AwardXPRequest{CampaignId: campaign, TreasurePointIds: keys(n)}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if reason, field := check(t, tc.build(100)); reason != 0 {
				t.Errorf("100 items: refused %v at %q", reason, field)
			}
			if reason, field := check(t, tc.build(101)); reason != listLimit || field != tc.field {
				t.Errorf("101 items: refused %v at %q, want %v at %q", reason, field, listLimit, tc.field)
			}
		})
	}
}

// A text of 2000 characters (up to 4 bytes each) and a personality of 1000 fit
// the default of a string.
func TestTextsOf2000CharactersFitTheDefault(t *testing.T) {
	story := &charactersv1.UpdateCharacterStoryRequest{Story: &charactersv1.CharacterStory{
		Allies:      strings.Repeat(emoji, 2000),
		Appearance:  &charactersv1.Appearance{Description: strings.Repeat(emoji, 2000)},
		Personality: &charactersv1.Personality{Traits: strings.Repeat(emoji, 1000)},
	}}
	if reason, field := check(t, story); reason != 0 {
		t.Errorf("refused %v at %q", reason, field)
	}
	point := &mapsv1.CreateMapPointRequest{Description: strings.Repeat(emoji, 2000)}
	if reason, field := check(t, point); reason != 0 {
		t.Errorf("point description refused %v at %q", reason, field)
	}
}
