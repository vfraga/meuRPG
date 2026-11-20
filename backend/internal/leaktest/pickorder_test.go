package leaktest

import (
	"bytes"
	"strings"
	"testing"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// closedKind is one kind of table entry a sheet takes, and the sheet of a player's draft that takes it: its skill list holds
// listed, and nothing in the draft lists unlisted.
type closedKind struct {
	name string
	// make creates the entry named name and says its key.
	make func(p *choicesProbe, name string) string
	// draft is the draft of a character that takes the entry and picks the skills.
	draft func(entry string, skills ...string) *charactersv1.CharacterSheet
}

const (
	listedSkill   = "skill:stealth"
	unlistedSkill = "skill:acrobatics"
)

func (p *choicesProbe) tableEntry(req *rulesv1.CreateTableEntryRequest) string {
	req.CampaignId = p.w.campaign
	res, err := p.w.master.table.CreateTableEntry(p.w.t.Context(), rq(req))
	if err != nil {
		p.w.t.Fatalf("creating the entry: %v", err)
	}
	return res.Msg.GetEntry().GetKey()
}

func skillChoice() *rulesv1.TableEffect {
	return &rulesv1.TableEffect{Type: "choice", Choice: "skill", Count: 1, From: []string{listedSkill, "skill:perception"}}
}

func closedKinds() []closedKind {
	return []closedKind{
		{
			name: "class",
			make: func(p *choicesProbe, name string) string {
				c := tableClass(name)
				c.SkillFrom = []string{"skill:arcana", "skill:history", listedSkill, "skill:perception"}
				return p.createClass(c)
			},
			draft: func(entry string, skills ...string) *charactersv1.CharacterSheet {
				s := sheet(entry, 1)
				s.GetFull().SkillProficiencyKeys = skills
				return s
			},
		},
		{
			name: "class feature",
			make: func(p *choicesProbe, name string) string {
				return p.createClass(tableClass(name, &rulesv1.TableFeature{NamePt: "Estudo", DescPt: []string{"Texto de teste."}, Effects: []*rulesv1.TableEffect{skillChoice()}}))
			},
			draft: func(entry string, skills ...string) *charactersv1.CharacterSheet {
				s := sheet(entry, 1)
				s.GetFull().SkillProficiencyKeys = skills
				return s
			},
		},
		{
			name: "race",
			make: func(p *choicesProbe, name string) string {
				r := fitRace(listedSkill, "skill:perception")
				r.NamePt = name
				return p.tableEntry(&rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableRace{TableRace: r}})
			},
			draft: func(entry string, skills ...string) *charactersv1.CharacterSheet {
				return fitSheet("class:wizard", entry, skills...)
			},
		},
		{
			name: "background",
			make: func(p *choicesProbe, name string) string {
				return p.tableEntry(&rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableBackground{TableBackground: &rulesv1.TableBackground{
					NamePt: name, Skills: []string{"skill:insight"},
					Feature: &rulesv1.TableFeature{NamePt: "Rede " + name, DescPt: []string{"Texto de teste."}},
					Groups:  []*rulesv1.TableBackgroundGroup{{Kind: rulesv1.TableBackgroundGroupKind_TABLE_BACKGROUND_GROUP_KIND_SKILL, Choose: 1, Options: []string{listedSkill, "skill:perception"}}},
				}}})
			},
			draft: func(entry string, skills ...string) *charactersv1.CharacterSheet {
				s := fitSheet("class:wizard", "race:human", skills...)
				s.GetFull().Background = &charactersv1.FullSheet_BackgroundKey{BackgroundKey: entry}
				return s
			},
		},
		{
			name: "feat",
			make: func(p *choicesProbe, name string) string {
				return p.tableEntry(&rulesv1.CreateTableEntryRequest{Body: &rulesv1.CreateTableEntryRequest_TableFeat{TableFeat: &rulesv1.TableFeat{
					NamePt: name, DescPt: []string{"Texto de teste."}, Effects: []*rulesv1.TableEffect{skillChoice()},
				}}})
			},
			draft: func(entry string, skills ...string) *charactersv1.CharacterSheet {
				s := fitSheet("class:wizard", "race:human", skills...)
				s.GetFull().FeatKeys = []string{entry}
				return s
			},
		},
	}
}

// A player's draft that takes an entry the table switched off or archived, which the stored sheet does not hold, is answered as
// that entry is answered with no skill picked: the closed entry comes first, whatever the skills picked, the one its list holds
// and the one no list holds. The level-up of a class the table keeps out answers the same way.
func TestAClosedEntryIsAnsweredFirstWhateverSkillIsPicked(t *testing.T) {
	p := newChoicesProbe(t)
	w, ctx := p.w, t.Context()
	must(w.master.campaigns.SetCampaignXpMode(ctx, rq(&campaignsv1.SetCampaignXpModeRequest{CampaignId: w.campaign, XpMode: campaignsv1.XpMode_XP_MODE_ENEMIES})))
	stored := sheet("class:wizard", 1)
	stored.GetFull().ExperiencePoints = 6500 // enough for the level-up rows
	held := must(w.ana.characters.CreateCharacter(ctx, rq(&charactersv1.CreateCharacterRequest{
		CampaignId: w.campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Name: "Anã", Sheet: stored,
	}))).GetCharacter()
	levelUpInto := func(class string, skills ...string) *charactersv1.LevelUpChoices {
		return &charactersv1.LevelUpChoices{
			ClassKey: class, SkillProficiencyKeys: skills,
			HitPoints: &charactersv1.LevelUpHitPoints{Method: charactersv1.LevelUpHitPointsMethod_LEVEL_UP_HIT_POINTS_METHOD_AVERAGE},
		}
	}
	player := charactersv1.CharacterKind_CHARACTER_KIND_PLAYER

	type closing struct {
		how  string
		shut func(key string)
	}
	closings := []closing{
		{"switched off", func(key string) {
			must(w.master.table.SetOptionSwitches(ctx, rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: []*rulesv1.OptionSwitch{{Key: key, Off: true}}})))
		}},
		{"archived", func(key string) {
			must(w.master.table.ArchiveTableEntry(ctx, rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: key})))
		}},
	}
	for _, kind := range closedKinds() {
		for _, closing := range closings {
			entry := kind.make(p, "Zorvak "+kind.name+" "+closing.how)
			closing.shut(entry)
			calls := map[string]func(skills ...string) reply{
				"create": func(skills ...string) reply {
					return w.caio.call(charactersv1connect.CharacterServiceCreateCharacterProcedure, &charactersv1.CreateCharacterRequest{
						CampaignId: w.campaign, Kind: player, Name: "Nova", Sheet: kind.draft(entry, skills...),
					})
				},
				"update": func(skills ...string) reply {
					return w.ana.call(charactersv1connect.CharacterServiceUpdateCharacterProcedure, &charactersv1.UpdateCharacterRequest{
						CampaignId: w.campaign, CharacterId: held.GetId(), Revision: held.GetRevision(), Name: held.GetName(), Sheet: kind.draft(entry, skills...),
					})
				},
				"preview of a new sheet": func(skills ...string) reply {
					return w.caio.call(charactersv1connect.CharacterServicePreviewCharacterProcedure, &charactersv1.PreviewCharacterRequest{
						CampaignId: w.campaign, Kind: player, Sheet: kind.draft(entry, skills...),
					})
				},
				"preview of a stored sheet": func(skills ...string) reply {
					return w.ana.call(charactersv1connect.CharacterServicePreviewCharacterProcedure, &charactersv1.PreviewCharacterRequest{
						CampaignId: w.campaign, CharacterId: held.GetId(), Kind: player, Sheet: kind.draft(entry, skills...),
					})
				},
				"choices of a new sheet": func(skills ...string) reply {
					return w.caio.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
						CampaignId: w.campaign, Kind: player, Sheet: kind.draft(entry, skills...),
					})
				},
				"choices of a stored sheet": func(skills ...string) reply {
					return w.ana.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
						CampaignId: w.campaign, CharacterId: held.GetId(), Kind: player, Sheet: kind.draft(entry, skills...),
					})
				},
			}
			if kind.name == "class" || kind.name == "class feature" {
				calls["level-up preview"] = func(skills ...string) reply {
					return w.ana.call(charactersv1connect.CharacterServicePreviewLevelUpProcedure, &charactersv1.PreviewLevelUpRequest{
						CampaignId: w.campaign, CharacterId: held.GetId(), Choices: levelUpInto(entry, skills...),
					})
				}
				calls["level-up"] = func(skills ...string) reply {
					return w.ana.call(charactersv1connect.CharacterServiceLevelUpCharacterProcedure, &charactersv1.LevelUpCharacterRequest{
						CampaignId: w.campaign, CharacterId: held.GetId(), Revision: held.GetRevision(), Choices: levelUpInto(entry, skills...),
					})
				}
			}
			for call, send := range calls {
				none, listed, unlisted := send(), send(listedSkill), send(unlistedSkill)
				// The answer with no skill picked is the closed entry's (main's): its own refusal, naming its key.
				reason := "CHARACTER_BLOCKED_REASON_SWITCHED_OFF_CONTENT"
				switch {
				case strings.HasPrefix(call, "level-up") && closing.how == "archived":
					reason = "LEVEL_UP_REFUSAL_REASON_ARCHIVED_CHOICE"
				case strings.HasPrefix(call, "level-up"):
					reason = "LEVEL_UP_REFUSAL_REASON_SWITCHED_OFF_CHOICE"
				case closing.how == "archived":
					reason = "CHARACTER_BLOCKED_REASON_ARCHIVED_CONTENT"
				}
				if none.ok() || !strings.Contains(string(none.body), reason) || !strings.Contains(string(none.body), entry) {
					t.Errorf("%s, %s, %s: the draft with no skill picked is answered HTTP %d %s, want the refusal of a closed entry (%s)", kind.name, closing.how, call, none.status, shorten(none.body), reason)
				}
				for what, got := range map[string]reply{"a skill the entry's list holds": listed, "a skill no list holds": unlisted} {
					if got.status != none.status || !bytes.Equal(got.body, none.body) {
						t.Errorf("%s, %s, %s, %s: HTTP %d %s\nwith no skill picked: HTTP %d %s", kind.name, closing.how, call, what, got.status, shorten(got.body), none.status, shorten(none.body))
					}
				}
			}
		}
	}
}
