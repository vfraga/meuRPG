package characters

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"
)

// Every function of the package that builds a rules.Build from a sheet or derives one is listed here with what it does about
// the infusion records of the character (Build.Infusions): "loads" (it reads them, or takes them as an argument), "given"
// (the build comes in, and the text names the caller that loads it) or "none" (it reads no number an infusion changes, and
// the text says what it reads). A function that builds or derives and is not listed fails the test, so a new site makes its
// author decide; a "loads" site that no longer reads the records fails it too.
var infusionSites = map[string]string{
	// The sheet and the checks of a write.
	"Service.characterToProto":       "loads: the records come in with the row",
	"Service.previewDerived":         "loads: the records of the character the preview is for",
	"refuseInfusedBonus":             "loads: both builds carry the records of the save",
	"Service.attuneIn":               "loads: the records of the locked character",
	"Service.newLevelUpTarget":       "loads: the records of the character, kept in the target's build",
	"levelUpTarget.buildOf":          "loads: the target's records on the planned sheet",
	"Service.PreviewLevelUp":         "loads: the planned sheet is built with the target's records",
	"checkPlannedLevel":              "loads: the planned sheet is built with the target's records",
	"Service.fullFighter":            "loads: the combat's armor class and attacks",
	"Service.infusionViewOf":         "loads: the records of the character the infusion call works on",
	"infusionView.checkItem":         "given: the view's build carries the records",
	"infusionView.targets":           "given: the view's build carries the records",
	"infusionView.moveKeepsValid":    "given: the view's build carries the records; the move is applied to them",
	"knownInfusionKeys":              "none: it derives a build of its own whose only records are the probes it makes",
	"askOfEntry":                     "given: attuneIn passes the build with the records",
	"entryStands":                    "given: attuneIn passes the build with the records",
	"derive":                         "loads: the records come in as an argument; the callers that read no infusion pass nil",
	"Service.infusedBuild":           "loads: the build with the records of the character",
	"buildWithStates":                "given: the loaders set Build.Infusions after it",
	"mageArmorOf":                    "given: MageArmorAC builds with infusedBuild",
	"Service.endInfusionsOfGoneGear": "none: it asks which gear keys the sheet lists",

	// Callers of derive that read no number an infusion changes.
	"combatCharacter": "none: it reads the initiative, the speeds, the size, the hit points and the jumps",
	"partyMember":     "none: it reads the senses and the passive Perception",

	// The checks of a new pick.
	"unknownToolPick":       "none: it reads the keys of the picks and the owner of a tool pick, no number an infusion changes",
	"refuseUnknownNewTools": "none: it reads the keys of the picks and the owner of a tool pick, no number an infusion changes",

	// Reads of the sheet that read a number no infusion changes.
	"maxima":                      "none: it reads the hit points, the hit dice, the slots, the resources and the states",
	"fullReactionStats":           "none: it reads whether a shield counts, from the equipped keys",
	"Service.ownerEnv":            "none: the formula environment has the level, the modifiers, the proficiency, the armor category, the shield and the speed",
	"ownerEnvOf":                  "given: ownerEnv; the environment reads no infusion",
	"Service.CheckSummon":         "none: what a spell may summon reads the class, the level and the spells",
	"Service.readSummonOptions":   "none: what a spell may summon reads the class, the level and the spells",
	"Service.summonCharacter":     "none: the build of the summoner's class, level and spells",
	"Service.Party":               "none: it reads the total level and the XP",
	"Service.PartyLevels":         "none: it reads the total level",
	"Service.checkLevelUpDue":     "none: it reads the total level and the XP of the next one",
	"Service.learnableLists":      "none: it reads the spell lists of the classes",
	"Service.endRestPicks":        "none: it reads the daily choices",
	"Service.endGivenUpOptions":   "none: it reads the sources of the resources and states a level-up's given-up options defined, and no infusion defines one",
	"mayNeedRestState":            "none: it reads whether a rest binds a choice",
	"withDailyPick":               "none: it reads the daily choices",
	"Service.refreshKnownIssues":  "none: the issues of a sheet come from its keys, and no infusion raises one",
	"checkFullSheet":              "none: validation and the known issues read the sheet, and no infusion raises one",
	"characterList.describeSheet": "none: it reads the class summary and the open choices",
	"gearChangeOf":                "none: it compares the armor and shield worn",
	"Service.setCarried":          "none: it checks the carried keys; moving a piece between equipped and carried ends no record",
	"Service.AssumeWildShape":     "none: it reads which forms the character may take",
	"Service.ListWildShapeForms":  "none: it reads which forms the character may take",
	"extraBonusesOver":            "none: it reads the ability scores",
	"applyLevelUp":                "none: it reads the hit points method of the sheet it copies",

	// The choices and the table's content: they read the keys the sheet holds.
	"CheckSheetCanMove":       "none: it reads the keys of the sheet",
	"Service.PreviewChoices":  "none: it reads the open choices",
	"Service.PreviewClaim":    "none: it reads the class summary",
	"Service.ownSheetKeys":    "none: it reads the keys of the sheet",
	"affectedSheets":          "none: it reads the keys of the sheets",
	"characterUses":           "none: it reads the keys of the sheets",
	"applyChoicePicksFor":     "none: it reads the choice engine's keys (option keys and the kept-out list) and only appends feature-choice keys and texts; Choices() derives a Build with no Infusions, and applyInfusions only adds item bonuses to AC, attacks and the spell attack, which none of these helpers read",
	"newKeptOutPicks":         "none: it compares the choice engine's kept-out keys with the stored feature_choice_keys; Choices() derives a Build with no Infusions, and applyInfusions only adds item bonuses to AC, attacks and the spell attack, which none of these helpers read",
	"newKeptOutField":         "none: it names a field from the rules' build keys (rules.BuildKeys); Choices() derives a Build with no Infusions, and applyInfusions only adds item bonuses to AC, attacks and the spell attack, which none of these helpers read",
	"refuseUnfitPickIn":       "none: it places the sheet's skill, expertise and tool picks in their sources; infusions only add item bonuses to AC, attacks and the spell attack, which no source reads",
	"carryFlags":              "none: it reads the flags of the table's content",
	"issuesTiedTo":            "none: it reads the flags of the table's content",
	"newArchivedChoice":       "none: it reads the keys of the sheet",
	"newOffChoice":            "none: it reads the keys of the sheet",
	"openChoicesOf":           "none: it reads the open choices",
	"pendingChoiceCount":      "none: it reads the open choices",
	"refuseChoices":           "none: it reads the open choices",
	"refuseNewChoiceProblems": "none: it reads the open choices",
	"sheetContentKeys":        "none: it reads the keys of the sheet",
	"playerUses":              "none: it reads the keys of the sheets",
	"unmetMulticlass":         "none: it reads the classes and their prerequisites",
}

// infusionEvidence are the names a "loads" function uses that show it reads the records.
var infusionEvidence = []string{"holderInfusions", "withInfusions", "infusedBuild", "Infusions", "infusions"}

// ownFiles parses the package's own files, the tests left out.
func ownFiles(t *testing.T) []*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the package: %v", err)
	}
	var files []*ast.File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		files = append(files, f)
	}
	return files
}

// funcName is "Name" for a function and "Type.Name" for a method.
func funcName(fd *ast.FuncDecl) string {
	if fd.Recv == nil {
		return fd.Name.Name
	}
	rt := fd.Recv.List[0].Type
	if s, ok := rt.(*ast.StarExpr); ok {
		rt = s.X
	}
	if id, ok := rt.(*ast.Ident); ok {
		return id.Name + "." + fd.Name.Name
	}
	return fd.Name.Name
}

// looksAtInfusions says the function builds or derives a sheet, and whether it shows the evidence of reading the records.
// A method call named buildOf (the planned sheet's build of a level-up target) builds with the records it holds.
func looksAtInfusions(fd *ast.FuncDecl) (builds, shows bool) {
	shows = slices.ContainsFunc(fd.Type.Params.List, func(p *ast.Field) bool {
		return slices.ContainsFunc(p.Names, func(n *ast.Ident) bool { return n.Name == "infusions" })
	})
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			b, s := callBuilds(x)
			builds, shows = builds || b, shows || s
		case *ast.Ident:
			shows = shows || slices.Contains(infusionEvidence, x.Name)
		case *ast.SelectorExpr:
			shows = shows || slices.Contains(infusionEvidence, x.Sel.Name)
		}
		return true
	})
	return builds, shows
}

// callBuilds says the call builds a sheet's Build or derives it, and whether it is a method that builds with its own records.
func callBuilds(c *ast.CallExpr) (builds, method bool) {
	switch fn := c.Fun.(type) {
	case *ast.Ident:
		return fn.Name == "buildOf" || fn.Name == "buildWithStates" || fn.Name == "derive", false
	case *ast.SelectorExpr:
		if pk, ok := fn.X.(*ast.Ident); ok && pk.Name == "rules" {
			return fn.Sel.Name == "Derive" || fn.Sel.Name == "DeriveGear", false
		}
		return fn.Sel.Name == "buildOf", fn.Sel.Name == "buildOf"
	}
	return false, false
}

// buildersOf returns, for every function of the package's own files, whether it builds or derives and whether it shows the
// evidence of reading the records.
func buildersOf(t *testing.T) (builders, evident map[string]bool) {
	t.Helper()
	builders, evident = map[string]bool{}, map[string]bool{}
	for _, f := range ownFiles(t) {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			builds, shows := looksAtInfusions(fd)
			if builds {
				builders[funcName(fd)] = true
			}
			evident[funcName(fd)] = shows
		}
	}
	return builders, evident
}

func TestEveryBuildOfASheetSaysWhatItDoesAboutTheInfusionRecords(t *testing.T) {
	t.Parallel()
	builders, evident := buildersOf(t)
	for name := range builders {
		if _, ok := infusionSites[name]; !ok {
			t.Errorf("%s builds or derives a sheet and is not in infusionSites: say whether it loads the infusion records or why it needs none", name)
		}
	}
	for name, how := range infusionSites {
		kind, why, ok := strings.Cut(how, ": ")
		switch {
		case !ok || strings.TrimSpace(why) == "" || !slices.Contains([]string{"loads", "given", "none"}, kind):
			t.Errorf("%s: %q is not \"loads|given|none: <why>\"", name, how)
		case !builders[name]:
			t.Errorf("%s is listed but no longer builds or derives a sheet", name)
		case kind == "loads" && !evident[name]:
			t.Errorf("%s says it loads the infusion records and shows no sign of reading them", name)
		}
	}
}
