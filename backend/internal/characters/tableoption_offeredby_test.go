package characters

import (
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The feature that offers a table option is named to a player only when the player may read the feature (RN-23).

func TestAPlayerCopyOfAnOptionNamesNoHiddenOfferingFeature(t *testing.T) {
	t.Parallel()
	const offerer = "feature:class-guardiao-3f9a1c--vigor@mesa"
	e := entryRow{
		row:  charactersdb.CampaignContent{ContentKey: "option:table_feature:postura@mesa"},
		kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_OPTION,
		body: &rulesv1.TableOption{NamePt: "Postura", Set: "table_feature", OfferedBy: offerer},
	}
	offeredBy := func(r entryRow) string { return r.body.(*rulesv1.TableOption).GetOfferedBy() }

	// The master reads everything, so nothing is hidden.
	if got := offeredBy(forPlayer(e, nil, nil)); got != offerer {
		t.Errorf("master: offered_by = %q, want %q", got, offerer)
	}
	// A player of a table where the class is archived.
	hiddenAll := func(k string) bool { return k == offerer }
	if got := offeredBy(forPlayer(e, hiddenAll, nil)); got != "" {
		t.Errorf("player, feature hidden: offered_by = %q, want it blank", got)
	}
	if got := offeredBy(e); got != offerer {
		t.Errorf("the stored body changed: offered_by = %q", got)
	}
	// A player who may read the feature keeps it.
	none := func(string) bool { return false }
	if got := offeredBy(forPlayer(e, none, nil)); got != offerer {
		t.Errorf("player, feature visible: offered_by = %q, want %q", got, offerer)
	}
}

const (
	offererSubclass = "subclass:guarda-do-corvo@mesa"
	offererFeature  = "feature:truques-de-lamina@mesa"
	offererOption   = "option:table_feature:golpe-do-corvo@mesa"
	offererName     = "Truques de lâmina"
)

// offererContent is a table subclass of the SRD fighter whose level 3 feature lists one option, and a table option offered by that
// feature. The subclass is archived or switched off as the case says.
func offererContent(t *testing.T, archived, off bool) *rules.Content {
	t.Helper()
	srd, err := rules.LoadSRD()
	if err != nil {
		t.Fatal(err)
	}
	feature := rules.TableFeature{
		Key: offererFeature, NamePT: offererName, DescPT: []string{"Escolha."},
		Effects: []rules.Effect{{Type: "choice", Choice: "feature", Count: 1}},
		Options: []rules.TableFeature{{Key: "feature:corte-baixo@mesa", NamePT: "Corte baixo", DescPT: []string{"Um corte."}}},
	}
	o := rules.Overlay{
		Revision: 1,
		Subclasses: []rules.TableSubclass{{
			TableEntry: rules.TableEntry{Key: offererSubclass, NamePT: "Guarda do Corvo", Archived: archived}, Class: "class:fighter",
			Levels: []rules.TableSubclassLevel{{Level: 3, Features: []rules.TableFeature{feature}}},
		}},
		Options: []rules.TableOption{{TableEntry: rules.TableEntry{Key: offererOption, NamePT: "Golpe do Corvo"}, Set: "table_feature", OfferedBy: offererFeature, DescPT: []string{"Um golpe."}}},
	}
	if off {
		o.Off = []string{offererSubclass}
	}
	c, err := srd.With(o)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// The entries of the table as a listing reads them: the subclass and the option that names its feature.
func offererEntries() []entryRow {
	return []entryRow{
		{
			row:  charactersdb.CampaignContent{ContentKey: offererSubclass, NamePt: "Guarda do Corvo"},
			kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_SUBCLASS,
			body: &rulesv1.TableSubclass{NamePt: "Guarda do Corvo", ClassKey: "class:fighter", Levels: []*rulesv1.TableSubclassLevel{{Level: 3, Features: []*rulesv1.TableFeature{{Key: offererFeature, NamePt: offererName}}}}},
		},
		{
			row:  charactersdb.CampaignContent{ContentKey: offererOption, NamePt: "Golpe do Corvo"},
			kind: rulesv1.TableContentKind_TABLE_CONTENT_KIND_OPTION,
			body: &rulesv1.TableOption{NamePt: "Golpe do Corvo", Set: "table_feature", OfferedBy: offererFeature},
		},
	}
}

// read is the option of a listing, as JSON, and whether the entry is there.
func readOption(t *testing.T, l *tableListing) (offeredBy string, there bool, body string) {
	t.Helper()
	res := l.response()
	for _, e := range res.GetEntries() {
		if e.GetKey() == offererOption {
			offeredBy, there = e.GetTableOption().GetOfferedBy(), true
		}
	}
	return offeredBy, there, protojson.Format(res)
}

// A player whose sheet holds the option, and not its offering feature (nor the subclass that owns it), reads the option with no
// `offered_by`, and the answer names neither the feature's key nor its name. The master and a player whose sheet holds the
// subclass read the key.
func TestASheetHeldOptionNamesNoHiddenOfferingFeature(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		archived, off bool
	}{{"archived", true, false}, {"off", false, true}} {
		content := offererContent(t, tc.archived, tc.off)
		if !content.Hidden(offererFeature) {
			t.Fatalf("%s: the feature is not hidden", tc.name)
		}
		listing := func(master bool, mine ...string) *tableListing {
			l := &tableListing{master: master, content: content, entries: offererEntries(), mine: map[string]bool{}}
			if !master {
				l.hidden = content.Hidden
			}
			for _, k := range mine {
				l.mine[k] = true
			}
			return l
		}
		leaks := func(body string) bool {
			return strings.Contains(body, offererFeature) || strings.Contains(body, offererName)
		}
		if got, there, body := readOption(t, listing(true)); !there || got != offererFeature || !leaks(body) {
			t.Errorf("%s, master: offered_by = %q (entry there = %v), want the key and the name", tc.name, got, there)
		}
		if got, there, _ := readOption(t, listing(false, offererOption, offererSubclass)); !there || got != offererFeature {
			t.Errorf("%s, player whose sheet holds the subclass: offered_by = %q (entry there = %v), want the key", tc.name, got, there)
		}
		if got, there, body := readOption(t, listing(false, offererOption)); !there || got != "" || leaks(body) {
			t.Errorf("%s, player whose sheet holds only the option: offered_by = %q, entry there = %v, names the feature = %v; want the entry, no key, no name", tc.name, got, there, leaks(body))
		}
		// Without a sheet that holds it, the option is hidden with its offering feature: nothing of it reaches the player.
		if _, there, body := readOption(t, listing(false)); there || leaks(body) {
			t.Errorf("%s, player with no sheet: entry there = %v, names the feature = %v; want neither", tc.name, there, leaks(body))
		}
	}
}

// The listing of a real table: the option is the master's alone while its offering subclass is hidden, and a player reads both while it is live.
func TestAnOptionIsHiddenWithItsOfferingSubclassInTheRealListing(t *testing.T) {
	t.Parallel()
	tb := newChoiceTable(t)
	sub := tb.master.addEntry(t, tb.campaign, &rulesv1.TableSubclass{
		NamePt: "Guarda do Corvo", ClassKey: "class:fighter",
		Levels: []*rulesv1.TableSubclassLevel{{Level: 3, Features: []*rulesv1.TableFeature{{
			NamePt: offererName, Effects: []*rulesv1.TableEffect{{Type: "choice", Choice: "feature", Count: 1}},
			Options: []*rulesv1.TableFeature{{NamePt: "Corte baixo"}},
		}}}},
	})
	feature := sub.GetTableSubclass().GetLevels()[0].GetFeatures()[0]
	opt := tb.master.addEntry(t, tb.campaign, &rulesv1.TableOption{NamePt: "Golpe do Corvo", Set: "table_feature", OfferedBy: feature.GetKey()})

	list := func(u *user) (offeredBy string, there bool, body string) {
		t.Helper()
		res, err := u.table.ListTableEntries(t.Context(), connect.NewRequest(&rulesv1.ListTableEntriesRequest{CampaignId: tb.campaign}))
		if err != nil {
			t.Fatalf("ListTableEntries() error = %v", err)
		}
		for _, e := range res.Msg.GetEntries() {
			if e.GetKey() == opt.GetKey() {
				offeredBy, there = e.GetTableOption().GetOfferedBy(), true
			}
		}
		return offeredBy, there, protojson.Format(res.Msg)
	}
	if got, there, _ := list(tb.ana); !there || got != feature.GetKey() {
		t.Fatalf("live subclass, player: offered_by = %q (entry there = %v), want the key", got, there)
	}
	steps := []struct {
		name string
		hide func()
	}{
		{"archived", func() {
			if _, err := tb.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: tb.campaign, Key: sub.GetKey()})); err != nil {
				t.Fatalf("ArchiveTableEntry() error = %v", err)
			}
		}},
		{"off", func() {
			if _, err := tb.master.table.UnarchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.UnarchiveTableEntryRequest{CampaignId: tb.campaign, Key: sub.GetKey()})); err != nil {
				t.Fatalf("UnarchiveTableEntry() error = %v", err)
			}
			tb.master.setOff(t, tb.campaign, true, sub.GetKey())
		}},
	}
	for _, st := range steps {
		st.hide()
		if got, there, body := list(tb.master); !there || got != feature.GetKey() || !strings.Contains(body, offererName) {
			t.Errorf("%s, master: offered_by = %q (entry there = %v), want the key and the name", st.name, got, there)
		}
		if _, there, body := list(tb.ana); there || strings.Contains(body, feature.GetKey()) || strings.Contains(body, offererName) {
			t.Errorf("%s, player with no sheet: entry there = %v, want the option and the feature left out of the answer", st.name, there)
		}
	}
}
