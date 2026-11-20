package leaktest

import (
	"strings"
	"testing"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The table's switches apply to every spell choice of a character that is already stored: the previews of Ana's
// character, asked by every kind of person, with a pick of a spell the master switched off and with a pick of a spell of
// the table the master retired. The master's answer is the control.
func TestTheSwitchesApplyToEverySpellChoiceOfAStoredCharacterForEveryone(t *testing.T) {
	p := newSummonProbe(t)
	w := p.w
	const (
		tome = "feature:pact-of-the-tome"
		book = "feature:eldritch-invocation-book-of-ancient-secrets"
	)
	tableSpell := func(name string, level int32, ritual bool, class string) string {
		sp := &rulesv1.TableSpell{
			NamePt: name, Level: level, Ritual: ritual, SchoolKey: "school:evocation",
			CastingTime: &rulesv1.TableSpellCastingTime{Unit: rulesv1.CastingTimeUnit_CASTING_TIME_UNIT_ACTION, Amount: 1},
			Range:       &rulesv1.TableSpellRange{Kind: rulesv1.SpellRangeKind_SPELL_RANGE_KIND_SELF},
			Duration:    &rulesv1.TableSpellDuration{Kind: rulesv1.SpellDurationKind_SPELL_DURATION_KIND_INSTANTANEOUS},
			Components:  &rulesv1.TableSpellComponents{Verbal: true},
			ClassKeys:   []string{class}, DescPt: []string{"Um texto."},
			Target: &rulesv1.TableSpellTarget{Kind: rulesv1.TableSpellTargetKind_TABLE_SPELL_TARGET_KIND_SELF},
		}
		if ritual {
			sp.Duration = &rulesv1.TableSpellDuration{Kind: rulesv1.SpellDurationKind_SPELL_DURATION_KIND_TIMED, Amount: 10, Unit: rulesv1.SpellDurationUnit_SPELL_DURATION_UNIT_MINUTE}
		}
		entry := must(w.master.table.CreateTableEntry(t.Context(), rq(&rulesv1.CreateTableEntryRequest{
			CampaignId: w.campaign, Body: &rulesv1.CreateTableEntryRequest_TableSpell{TableSpell: sp},
		}))).GetEntry()
		return entry.GetKey()
	}
	rows := []struct {
		name, off, retired string
		race, subrace      string
		class              string
		level              int32
		picks              []string
		pick               string
	}{
		{"the book's rituals", "spell:detect-magic", tableSpell("Ritual da mesa", 1, true, "class:wizard"), "race:human", "", "class:warlock", 5, []string{tome, book, "feature:eldritch-invocation-devils-sight"}, book + "#rituals="},
		{"the Tome's cantrips", "spell:guidance", tableSpell("Truque da mesa", 0, false, "class:wizard"), "race:human", "", "class:warlock", 3, []string{tome}, tome + "#cantrips="},
		{"a Mystic Arcanum", "spell:eyebite", tableSpell("Arcana da mesa", 6, false, "class:warlock"), "race:human", "", "class:warlock", 11, []string{"feature:pact-of-the-chain"}, "feature:mystic-arcanum-6th-level="},
		{"a bonus cantrip", "spell:fire-bolt", tableSpell("Bônus da mesa", 0, false, "class:wizard"), "race:elf", "subrace:high-elf", "class:wizard", 1, nil, "trait:high-elf-cantrip="},
	}
	preview := func(who *person, row int, spell string) reply {
		r := rows[row]
		keys := append([]string{}, r.picks...)
		if spell != "" {
			keys = append(keys, r.pick+spell)
		}
		return who.call(charactersv1connect.CharacterServicePreviewChoicesProcedure, &charactersv1.PreviewChoicesRequest{
			CampaignId: w.campaign, CharacterId: w.pens.GetId(), Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER,
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
		reads  bool // the character is Ana's: she and the master read it
	}
	people := []who{
		{"the master", w.master, true},
		{"Ana, the owner", w.ana, true},
		{"Caio, another player", w.caio, false},
		{"a member who waits for approval", w.pending, false},
		{"someone outside the campaign", w.stranger, false},
	}
	names := func(r reply, spell string) bool { return strings.Contains(string(r.body), spell) }
	before := map[string]reply{}
	for i, r := range rows {
		for _, who := range people {
			for _, spell := range []string{"", r.off, r.retired} {
				got := preview(who.person, i, spell)
				before[r.name+who.name+spell] = got
				if who.reads != got.ok() {
					t.Errorf("%s, before the switch, %s, with %q: HTTP %d, want a read only for Ana and the master (the control)", r.name, who.name, spell, got.status)
				}
			}
		}
	}
	var switches []*rulesv1.OptionSwitch
	for _, r := range rows {
		switches = append(switches, &rulesv1.OptionSwitch{Key: r.off, Off: true})
		must(w.master.table.ArchiveTableEntry(t.Context(), rq(&rulesv1.ArchiveTableEntryRequest{CampaignId: w.campaign, Key: r.retired})))
	}
	must(w.master.table.SetOptionSwitches(t.Context(), rq(&rulesv1.SetOptionSwitchesRequest{CampaignId: w.campaign, Switches: switches})))
	for i, r := range rows {
		for _, who := range people {
			t.Run(r.name+"/"+who.name, func(t *testing.T) {
				without := preview(who.person, i, "")
				off, retired := preview(who.person, i, r.off), preview(who.person, i, r.retired)
				switch {
				case who.person == w.master:
					if !without.ok() || !off.ok() || !names(off, `"`+r.off+`"`) {
						t.Errorf("the master's previews: HTTP %d and %d, want the switched-off spell read (the control)", without.status, off.status)
					}
					if retired.ok() {
						t.Errorf("the master's preview that picks the retired spell: HTTP %d, want it refused", retired.status)
					}
				case who.reads:
					if !without.ok() || names(without, r.off) || names(without, r.retired) {
						t.Errorf("Ana's preview without the pick: HTTP %d, names a spell the table closed: %v", without.status, shorten(without.body))
					}
					if off.ok() || retired.ok() {
						t.Errorf("Ana's previews that pick them: HTTP %d and %d, want both refused", off.status, retired.status)
					}
				default:
					for _, spell := range []string{"", r.off, r.retired} {
						got := preview(who.person, i, spell)
						if got.ok() || got.status != before[r.name+who.name+spell].status || names(got, r.off) || names(got, r.retired) {
							t.Errorf("with %q: HTTP %d %s; want the answer it had before the switch, naming nothing", spell, got.status, shorten(got.body))
						}
					}
				}
			})
		}
	}
}
