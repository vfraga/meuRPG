package characters

import (
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// A skill the sheet has from its background reads already_have on a level's skill source, as on the multiclass offer: the
// mark is the sheet's derived skills at full proficiency, not only the picks the sheet lists.
func TestASkillTheBackgroundGrantsReadsAlreadyHaveOnALevelsSource(t *testing.T) {
	t.Parallel()
	c := loadSRD(t)
	b := wizardBuild("", false) // human acolyte: the background grants insight and religion
	if has := b.SkillProficiencies; len(has) != 0 {
		t.Fatalf("the build lists its picks %v, want none", has)
	}
	src := []rules.LevelSkillSource{{Owner: "class:wizard", Count: 1, From: []string{"skill:insight", "skill:arcana"}}}
	got := skillSourcesToProto(c, rules.Derive(b, c).Skills, src)
	if len(got) != 1 || len(got[0].GetFrom()) != 2 {
		t.Fatalf("sources = %v", got)
	}
	for _, o := range got[0].GetFrom() {
		want := o.GetKey() == "skill:insight"
		if o.GetAlreadyHave() != want {
			t.Errorf("already_have(%s) = %v, want %v", o.GetKey(), o.GetAlreadyHave(), want)
		}
	}
}
