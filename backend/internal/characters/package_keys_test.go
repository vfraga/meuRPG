package characters

import (
	"strings"
	"testing"
	"time"

	pkgv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaignpackage/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/campaignpackage"
)

// packageBodies are valid bodies of one kind of table content, named for the key the server makes
// of them. An option comes in two sets. keys holds the keys of the entries made before it, by kind
// prefix: a subclass names its class, a subrace its race and a spell its class.
func packageBodies(prefix string, keys map[string]string) []tableBody {
	switch prefix {
	case "class":
		return []tableBody{testClass("Andarilho")}
	case "subclass":
		return []tableBody{testSubclass("Caminho do Vale", keys["class"])}
	case "race":
		return []tableBody{testRace("Povo do Vale")}
	case "subrace":
		return []tableBody{testSubrace("Povo da Serra", keys["race"])}
	case "background":
		return []tableBody{testBackground("Guarda de Farol")}
	case "spell":
		return []tableBody{testSpell("Raio de Teste", keys["class"])}
	case "feat":
		return []tableBody{testFeat("Vigilante")}
	case "option":
		return []tableBody{crowGaze(), &rulesv1.TableOption{NamePt: "Luta às Cegas", Set: "fighting_style", DescPt: []string{"Luta sem enxergar."}}}
	case "classfeature":
		return []tableBody{trackerGaze()}
	case "creature":
		return []tableBody{tableWolf()}
	case "weapon":
		return []tableBody{testWeapon("Espada do Vigia")}
	}
	return nil
}

// packageEntryOf is the entry of a body under a key, as a package carries it.
func packageEntryOf(kind rulesv1.TableContentKind, key string, body tableBody) *rulesv1.TableEntry {
	te := &rulesv1.TableEntry{Key: key, Kind: kind, NamePt: body.GetNamePt()}
	setBody(te, body)
	return te
}

// stageOneEntry stages an entry alone and says what the package would report.
func stageOneEntry(t *testing.T, te *rulesv1.TableEntry) (bool, []*pkgv1.PackageProblem) {
	t.Helper()
	in := campaignpackage.NewImport(packagePartOpen(t, nil), "00000000-0000-4000-8000-0000000000c1", "u", false, nil)
	_, ok := (&packagePart{s: &Service{now: time.Now}}).stageEntry(in, te, time.Now(), map[string]bool{})
	return ok, in.Problems.List
}

// A campaign package carries every key the server makes for every kind of table content: an entry is
// staged under the key its kind gets when the table creates it, an option's key naming its set.
func TestAPackageStagesTheKeyTheServerMakesForEveryTableKind(t *testing.T) {
	t.Parallel()
	keys := map[string]string{}
	for _, tk := range tableKinds {
		bodies := packageBodies(tk.prefix, keys)
		if len(bodies) == 0 {
			t.Fatalf("no body for the table kind %q", tk.prefix)
		}
		for _, body := range bodies {
			key := entryKey(tk.kind, body, nil)
			keys[tk.prefix] = key
			if ok, problems := stageOneEntry(t, packageEntryOf(tk.kind, key, body)); !ok {
				t.Errorf("the package refuses %q (kind %s): problems = %v", key, tk.prefix, problems)
			}
		}
	}
}

// An entry key is checked by the rule of its kind: "<kind>:<slug>@mesa" with a slug of a-z, 0-9 and
// hyphens that the engine does not reserve, and for an option "option:<set>:<slug>@mesa" with a known set.
func TestAPackageRefusesAnEntryKeyTheServerNeverMakes(t *testing.T) {
	t.Parallel()
	feat, option := rulesv1.TableContentKind_TABLE_CONTENT_KIND_FEAT, rulesv1.TableContentKind_TABLE_CONTENT_KIND_OPTION
	for _, tc := range []struct {
		name string
		kind rulesv1.TableContentKind
		key  string
		body tableBody
	}{
		{"no suffix", feat, "feat:vigia-do-farol", testFeat("Vigia")},
		{"another kind's prefix", feat, "monster:vigia@mesa", testFeat("Vigia")},
		{"a prefix that is not the body's kind", feat, "class:vigia@mesa", testFeat("Vigia")},
		{"an upper case letter", feat, "feat:Vigia@mesa", testFeat("Vigia")},
		{"an empty slug", feat, "feat:@mesa", testFeat("Vigia")},
		{"a slug past the limit", feat, "feat:" + strings.Repeat("a", 61) + "@mesa", testFeat("Vigia")},
		{"a reserved word in the slug", feat, "feat:wild-shape@mesa", testFeat("Vigia")},
		{"an option with no set", option, "option:vigia-do-farol@mesa", crowGaze()},
		{"an option of an unknown set", option, "option:nenhum:vigia-do-farol@mesa", crowGaze()},
		{"an option with an empty slug", option, "option:invocation:@mesa", crowGaze()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ok, problems := stageOneEntry(t, packageEntryOf(tc.kind, tc.key, tc.body))
			if ok || len(problems) != 1 || problems[0].GetReason() != pkgv1.PackageProblemReason_PACKAGE_PROBLEM_REASON_INVALID {
				t.Errorf("staging %q: ok = %v, problems = %v; want one INVALID problem", tc.key, ok, problems)
			}
		})
	}
}
