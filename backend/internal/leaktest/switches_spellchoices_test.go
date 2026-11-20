package leaktest

import (
	"strings"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The table's switches apply to every spell choice: the book's rituals, the Pact of the Tome's cantrips, a Mystic
// Arcanum and a bonus cantrip. Every kind of person previews a sheet with the pick and one without it, before and after
// the master's switch; the master's answer is the control.
func TestTheSwitchesApplyToEverySpellChoiceForEveryone(t *testing.T) {
	p := newSummonProbe(t)
	w := p.w
	const (
		tome = "feature:pact-of-the-tome"
		book = "feature:eldritch-invocation-book-of-ancient-secrets"
		inv  = "feature:eldritch-invocation-"
	)
	rows := []struct {
		name, spell string
		race        string
		subrace     string
		class       string
		level       int32
		picks       []string
		pick        string
	}{
		{"the book's rituals", "spell:detect-magic", "race:human", "", "class:warlock", 5, []string{tome, book, inv + "devils-sight"}, book + "#rituals="},
		{"the Tome's cantrips", "spell:guidance", "race:human", "", "class:warlock", 3, []string{tome}, tome + "#cantrips="},
		{"a Mystic Arcanum", "spell:eyebite", "race:human", "", "class:warlock", 11, []string{"feature:pact-of-the-chain"}, "feature:mystic-arcanum-6th-level="},
		{"a bonus cantrip", "spell:fire-bolt", "race:elf", "subrace:high-elf", "class:wizard", 1, nil, "trait:high-elf-cantrip="},
	}
	preview := func(who *person, kind charactersv1.CharacterKind, row int, withPick bool) reply {
		r := rows[row]
		keys := append([]string{}, r.picks...)
		if withPick {
			keys = append(keys, r.pick+r.spell)
		}
		return who.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
			CampaignId: w.campaign, Kind: kind,
			Sheet: &charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: &charactersv1.FullSheet{
				BaseScores: &rulesv1.AbilityScores{Strength: 10, Dexterity: 14, Constitution: 12, Intelligence: 16, Wisdom: 10, Charisma: 8},
				RaceKey:    r.race, SubraceKey: r.subrace, Classes: []*charactersv1.ClassLevel{{ClassKey: r.class, Level: r.level}},
				FeatureChoiceKeys: keys,
			}}},
		})
	}
	type who struct {
		name   string
		person *person
		kind   charactersv1.CharacterKind
		member bool
	}
	people := []who{
		{"the master", w.master, charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, true},
		{"Ana", w.ana, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, true},
		{"Caio", w.caio, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, true},
		{"a member who waits for approval", w.pending, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, true},
		{"someone outside the campaign", w.stranger, charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, false},
	}
	for i, r := range rows {
		for _, who := range people {
			for _, withPick := range []bool{false, true} {
				if got := preview(who.person, who.kind, i, withPick); who.member != got.ok() || (who.member && !strings.Contains(string(got.body), r.spell)) {
					t.Errorf("%s, before the switch, %s, with the pick %v: HTTP %d, names the spell %v; want a member to read it (the control)",
						r.name, who.name, withPick, got.status, strings.Contains(string(got.body), r.spell))
				}
			}
		}
	}
	var switches []*rulesv1.OptionSwitch
	for _, r := range rows {
		switches = append(switches, &rulesv1.OptionSwitch{Key: r.spell, Off: true})
	}
	must(w.master.table.SetOptionSwitches(t.Context(), rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: switches})))
	for i, r := range rows {
		for _, who := range people {
			t.Run(r.name+"/"+who.name, func(t *testing.T) {
				without := preview(who.person, who.kind, i, false)
				names := strings.Contains(string(without.body), r.spell)
				with := preview(who.person, who.kind, i, true)
				if who.person == w.master {
					if !without.ok() || !names || !with.ok() || !strings.Contains(string(with.body), `"`+r.spell+`"`) {
						t.Errorf("the master's preview: HTTP %d and %d, names the spell %v; want both to read it", without.status, with.status, names)
					}
					return
				}
				if names {
					t.Errorf("the preview names the spell: HTTP %d %s", without.status, shorten(without.body))
				}
				if with.ok() {
					t.Errorf("the answer differs from the control: HTTP %d %s", with.status, shorten(with.body))
				}
			})
		}
	}
}
