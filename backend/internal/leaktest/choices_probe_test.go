package leaktest

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The rows of the choices a table writes (a filter of proficiency kinds, the options of another feature): the same
// readers preview the same sheets, and the answers are compared whole.

// choicesProbe is a campaign with the master, Ana, Caio, a member who waits for approval and someone outside.
type choicesProbe struct {
	w *world
}

func newChoicesProbe(t *testing.T) *choicesProbe {
	t.Helper()
	w := &world{stack: newStack(t), secrets: newSecrets(), pts: map[string]*mapsv1.MapPoint{}, puzzles: map[string]*playv1.Puzzle{}, notes: map[string]string{}}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("building the fixture: %v", r)
		}
	}()
	w.buildPeople()
	return &choicesProbe{w: w}
}

// reader is a person who previews choices, and whether the campaign lets them in.
type reader struct {
	name   string
	person *person
	kind   charactersv1.CharacterKind
	member bool
}

func (p *choicesProbe) readers() []reader {
	w := p.w
	return []reader{
		{"the master", w.master, charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, true},
		{"Ana", w.ana, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, true},
		{"Caio, another player", w.caio, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, true},
		{"a member who waits for approval", w.pending, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, true},
		{"someone outside the campaign", w.stranger, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, false},
	}
}

// tableClass is a table class with the features at its first level (the rest of its rows plain).
func tableClass(name string, first ...*rulesv1.TableFeature) *rulesv1.TableClass {
	c := &rulesv1.TableClass{
		NamePt: name, HitDie: 8,
		SavingThrows: []rulesv1.Ability{rulesv1.Ability_ABILITY_CONSTITUTION, rulesv1.Ability_ABILITY_WISDOM},
		SkillChoose:  2, SkillFrom: []string{"skill:arcana", "skill:history", "skill:medicine", "skill:nature", "skill:survival"},
		Proficiencies: []string{"proficiency:light-armor", "proficiency:simple-weapons"}, SubclassLevel: 3,
	}
	for lvl := 1; lvl <= 20; lvl++ {
		row := &rulesv1.TableClassLevel{}
		if lvl == 1 {
			row.Features = first
		}
		c.Levels = append(c.Levels, row)
	}
	return c
}

func (p *choicesProbe) createClass(c *rulesv1.TableClass) string {
	return must(p.w.master.table.CreateTableEntry(p.w.t.Context(), rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: p.w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableClass{TableClass: c},
	}))).GetEntry().GetKey()
}

// sheet is a draft of a character of the class, at the level, with the picks.
func sheet(class string, level int32, picks ...string) *charactersv1.CharacterSheet {
	return &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
		BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8},
		RaceKey:    "race:human", Classes: []*charactersv1.ClassLevel{{ClassKey: class, Level: level}}, FeatureChoiceKeys: picks,
	}}}
}

// preview is the reader's PreviewChoices of a draft sheet.
func (p *choicesProbe) preview(who reader, s *charactersv1.CharacterSheet) reply {
	return who.person.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
		CampaignId: p.w.campaign, Kind: who.kind, Sheet: s,
	})
}

// choicesOf reads the answer of a PreviewChoices.
func choicesOf(t *testing.T, r reply) *charactersv1.PreviewChoicesResponse {
	t.Helper()
	var out charactersv1.PreviewChoicesResponse
	if err := protojson.Unmarshal(r.body, &out); err != nil {
		t.Fatalf("the answer %s is not a PreviewChoices: %v", shorten(r.body), err)
	}
	return &out
}

// optionsOf are the keys of the options of the choices whose key ends with the suffix, sorted.
func optionsOf(res *charactersv1.PreviewChoicesResponse, suffix string) []string {
	var out []string
	for _, g := range res.GetGroups() {
		for _, ch := range g.GetChoices() {
			if !strings.HasSuffix(ch.GetKey(), suffix) {
				continue
			}
			for _, o := range ch.GetOptions() {
				out = append(out, o.GetKey())
			}
		}
	}
	slices.Sort(out)
	return out
}

// sameAnswer says the two answers are one, but for the key each names. The encoded detail of an error holds the key too
// (with its length), so the comparison reads the plain form of the detail that comes with it.
func sameAnswer(a reply, keyA string, b reply, keyB string) bool {
	return a.status == b.status && keyless(a.body, keyA) == keyless(b.body, keyB)
}

// encodedDetail is the packed form of an error detail, which the same answer also gives in the clear (its debug field).
var encodedDetail = regexp.MustCompile(`"value":"[A-Za-z0-9_-]*",?`)

func keyless(body []byte, key string) string {
	return strings.ReplaceAll(encodedDetail.ReplaceAllString(string(body), ""), key, "K")
}

// createEntry creates the class and gives the entry.
func (p *choicesProbe) createEntry(t *testing.T, c *rulesv1.TableClass) *rulesv1.TableEntry {
	t.Helper()
	res, err := p.w.master.table.CreateTableEntry(t.Context(), rq(&rulesv1.CreateTableEntryRequest{
		CampaignId: p.w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableClass{TableClass: c},
	}))
	if err != nil {
		t.Fatalf("creating the class: %v", err)
	}
	return res.Msg.GetEntry()
}
