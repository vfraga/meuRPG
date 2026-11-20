package rules

import (
	"errors"
	"testing"
	"time"
)

// The derivation never applies more infusions than the host's limit at the class level: of the valid records, the newest keep
// their place and the oldest give way (since, then id), with a reason of their own. Nothing ends by itself. Every name is invented.

var infStart = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// infAt is the record with the infusion on the piece, made `days` days after infStart.
func infAt(id, infusion, key string, days int) InfusionRecord {
	r := infRec(id, infusion, key)
	r.Since = infStart.AddDate(0, 0, days)
	return r
}

// infGated is the overlay where the Gume option asks for the Couraca option to be picked.
func infGated() Overlay {
	o := infusionRulesOverlay()
	opts := o.Subclasses[0].Levels[0].Features[0].Options
	for i := range opts {
		if opts[i].Key == infGume {
			opts[i].Prerequisite = TableOptionPrerequisite{RequiresOptions: []string{infCouraca}}
		}
	}
	return o
}

// three is a sheet with three records at once: the oldest on the sword (a seal that asks for attunement, attuned), then an
// object and the newest on the axe.
func three(level int, picks ...string) Build {
	b := infBuild(level, picks...)
	b.Weapons = []string{infSword, infAxe}
	seal := infAt("a", infSelo, infSword, 0)
	seal.Attuned = true
	focus := infObject("b", infFoco, "Cajado")
	focus.Since, focus.Attuned = infStart.AddDate(0, 0, 1), true
	b.Infusions = []InfusionRecord{seal, focus, infAt("c", infGume, infAxe, 2)}
	return b
}

func TestTheRecordsBeyondTheLimitAreNotValidAndTheOldestGiveWay(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	b := three(5, infSelo, infFoco, infGume) // level 5 allows 2
	d := Derive(b, c)
	if s := infStatus(t, d, "a"); s.Valid || s.ReasonPT != reasonInfusionOver || s.Bonus != 0 || s.Counts {
		t.Errorf("the oldest = %+v, want not valid now: over the limit, no number", s)
	}
	for _, id := range []string{"b", "c"} {
		if s := infStatus(t, d, id); !s.Valid {
			t.Errorf("%s = %+v, want valid (the newest keep their place)", id, s)
		}
	}
	if len(d.InfusionHosts) != 1 || d.InfusionHosts[0].Limit != 2 || d.InfusionHosts[0].Used != 2 {
		t.Errorf("hosts = %+v, want limit 2, used 2", d.InfusionHosts)
	}
	if a, _ := attackOf(d, infSword); a.MagicBonus != 0 || a.InfusionNamePT != "" {
		t.Errorf("the sword of the record that gave way = %+v, want no bonus and no name", a)
	}
	if a, _ := attackOf(d, infAxe); a.MagicBonus != 1 {
		t.Errorf("the axe of the newest = %+v, want +1", a)
	}
	if got := AttunedCount(b, c); got != 1 {
		t.Errorf("attuned = %d, want 1 (the focus; the seal that gave way takes no place)", got)
	}
	if len(b.Infusions) != 3 {
		t.Errorf("the derivation changed the records: %d", len(b.Infusions))
	}
}

// A record the limit made give way is valid again when the limit grows back, and a level less is the lowered limit.
func TestALoweredLimitMakesTheOldestGiveWay(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	for _, tc := range []struct {
		level      int
		wantOldest bool
		attuned    int
	}{{6, true, 2}, {5, false, 1}} {
		b := three(tc.level, infSelo, infFoco, infGume)
		d := Derive(b, c)
		if got := infStatus(t, d, "a").Valid; got != tc.wantOldest {
			t.Errorf("level %d: the oldest valid = %v, want %v", tc.level, got, tc.wantOldest)
		}
		if got := AttunedCount(b, c); got != tc.attuned {
			t.Errorf("level %d: attuned = %d, want %d", tc.level, got, tc.attuned)
		}
	}
}

// A record that turns valid again (its prerequisite comes back) can leave more valid ones than the limit.
func TestARevivedRecordBeyondTheLimitMakesTheOldestGiveWay(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infGated())
	// The newest record's option asks for Couraca: lost, the two older ones fill the limit of 2.
	b := three(5, infSelo, infFoco, infGume)
	d := Derive(b, c)
	if s := infStatus(t, d, "c"); s.Valid || s.ReasonPT != reasonInfusionLost {
		t.Fatalf("the newest = %+v, want not valid now: lost", s)
	}
	for _, id := range []string{"a", "b"} {
		if s := infStatus(t, d, id); !s.Valid {
			t.Fatalf("%s = %+v, want valid while the newest is lost", id, s)
		}
	}
	if got := AttunedCount(b, c); got != 2 {
		t.Fatalf("attuned = %d, want 2", got)
	}
	// Couraca is picked: the prerequisite is met, the newest is valid again and the oldest gives way.
	b = three(5, infSelo, infFoco, infGume, infCouraca)
	d = Derive(b, c)
	if s := infStatus(t, d, "a"); s.Valid || s.ReasonPT != reasonInfusionOver {
		t.Errorf("the oldest = %+v, want not valid now: over the limit", s)
	}
	for _, id := range []string{"b", "c"} {
		if s := infStatus(t, d, id); !s.Valid {
			t.Errorf("%s = %+v, want valid", id, s)
		}
	}
	if got := AttunedCount(b, c); got != 1 {
		t.Errorf("attuned = %d, want 1", got)
	}
}

// Records that began together give way by id, the order the write uses.
func TestRecordsThatBeganTogetherGiveWayByID(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	b := infBuild(5, infTexto)
	b.Infusions = []InfusionRecord{infObject("2", infTexto, "B"), infObject("1", infTexto, "A"), infObject("3", infTexto, "C")}
	d := Derive(b, c)
	for id, want := range map[string]bool{"1": false, "2": true, "3": true} {
		if got := infStatus(t, d, id).Valid; got != want {
			t.Errorf("record %s valid = %v, want %v", id, got, want)
		}
	}
}

// A limit of other hosts is not shared: the count is per host, and a record that is not valid for another reason takes no place.
func TestOnlyValidRecordsFillTheLimit(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	b := infBuild(5, infTexto, infGume)
	b.Weapons = []string{infSword}
	b.Infusions = []InfusionRecord{
		infAt("a", infGume, infSword, 0),
		func() InfusionRecord {
			r := infObject("b", infCouraca, "Anel")
			r.Since = infStart.AddDate(0, 0, 1)
			return r
		}(), // not picked
		func() InfusionRecord {
			r := infObject("c", infTexto, "Lanterna")
			r.Since = infStart.AddDate(0, 0, 2)
			return r
		}(),
	}
	d := Derive(b, c)
	if !infStatus(t, d, "a").Valid || !infStatus(t, d, "c").Valid || infStatus(t, d, "b").ReasonPT != reasonInfusionUnpick {
		t.Errorf("statuses = %+v, want a and c valid and b not picked", d.Infusions)
	}
}

// The counts read what the sheet reads: a record that lost its prerequisite takes no attunement place and does not block a
// manual bonus on its piece.
func TestARecordThatLostItsPrerequisiteTakesNoAttunementAndBlocksNoManualBonus(t *testing.T) {
	t.Parallel()
	o := infusionRulesOverlay()
	opts := o.Subclasses[0].Levels[0].Features[0].Options
	for i := range opts {
		if opts[i].Key == infSelo {
			opts[i].Prerequisite = TableOptionPrerequisite{RequiresOptions: []string{infCouraca}}
		}
	}
	c := withOverlay(t, o)
	b := infBuild(5, infSelo, infGume)
	b.Weapons = []string{infSword, infAxe}
	seal := infAt("s", infSelo, infSword, 0)
	seal.Attuned = true
	b.Infusions = []InfusionRecord{seal, infAt("g", infGume, infAxe, 1)}
	if s := infStatus(t, Derive(b, c), "s"); s.Valid || s.ReasonPT != reasonInfusionLost {
		t.Fatalf("the seal = %+v, want not valid now: lost", s)
	}
	if got := AttunedCount(b, c); got != 0 {
		t.Errorf("attuned = %d, want 0 (a record that lost its prerequisite takes no place)", got)
	}
	b.ItemBonuses = []ItemBonus{{Key: infSword, Bonus: 1}, {Key: infAxe, Bonus: 1}}
	var ve *ValidationError
	if err := CheckManualBonuses(Build{}, b, c); !errors.As(err, &ve) || ve.Field != "full.item_bonuses[1].key" {
		t.Errorf("the manual bonuses: %v, want a refusal only at the axe (the sword's record lost its prerequisite)", err)
	}
}

// An option the master switched off after the pick still infuses: the record stays valid and gives its number.
func TestASwitchedOffOptionStillInfuses(t *testing.T) {
	t.Parallel()
	o := infusionRulesOverlay()
	o.Off = []string{infGume}
	c := withOverlay(t, o)
	b := infBuild(5, infGume)
	b.Weapons = []string{infSword}
	b.Infusions = []InfusionRecord{infRec("r1", infGume, infSword)}
	d := Derive(b, c)
	if s := infStatus(t, d, "r1"); !s.Valid || s.Bonus != 1 {
		t.Errorf("status = %+v, want valid with +1", s)
	}
	if a, _ := attackOf(d, infSword); a.MagicBonus != 1 {
		t.Errorf("the sword = %+v, want +1", a)
	}
}

// The check of a manual bonus reads the limit as the sheet does: a legacy piece with a manual bonus and an item bonus infusion
// is not valid on the sheet, so it takes no place of the limit, and the record under test is not pushed out by it.
func TestAManualBonusIsRefusedOnAPieceWhoseInfusionTheLimitKeepsOnTheSheet(t *testing.T) {
	t.Parallel()
	c := withOverlay(t, infusionRulesOverlay())
	b := infBuild(5, infGume, infTexto) // level 5 allows 2
	b.Weapons = []string{infSword, infAxe}
	b.Infusions = []InfusionRecord{
		infAt("axe", infGume, infAxe, 0),
		infAt("sword", infGume, infSword, 1), // the legacy piece: a manual bonus stands on it
		func() InfusionRecord {
			r := infObject("lamp", infTexto, "Lanterna")
			r.Since = infStart.AddDate(0, 0, 2)
			return r
		}(),
	}
	b.ItemBonuses = []ItemBonus{{Key: infSword, Bonus: 1}}
	d := Derive(b, c)
	if s := infStatus(t, d, "sword"); s.Valid || s.ReasonPT != reasonInfusionManual {
		t.Fatalf("the sword's record = %+v, want not valid: the manual bonus stands", s)
	}
	if s := infStatus(t, d, "axe"); !s.Valid {
		t.Fatalf("the axe's record = %+v, want valid on the sheet", s)
	}
	stored := b
	b.ItemBonuses = []ItemBonus{{Key: infSword, Bonus: 1}, {Key: infAxe, Bonus: 1}}
	var ve *ValidationError
	if err := CheckManualBonuses(stored, b, c); !errors.As(err, &ve) || ve.Field != "full.item_bonuses[1].key" {
		t.Errorf("the manual bonuses: %v, want a refusal at the axe, whose infusion the sheet keeps", err)
	}
}
