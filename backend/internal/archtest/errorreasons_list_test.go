package archtest

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaignpackage/v1"
	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/system/v1"
)

// reasonsFile is the closed list of everything the server can tell a client
// about why it refused (docs/architecture/api.md#error-reasons).
const reasonsFile = backendRoot + "/../proto/error-reasons.json"

type reasonList struct {
	Details    []detailEntry    `json:"details"`
	Body       []bodyEntry      `json:"body"`
	Violations []violationEntry `json:"violations"`
	Redirects  []redirectEntry  `json:"redirects"`
}

// detailEntry is a typed error detail (or a response message that carries
// the same kind of reason) and the enum fields that say why.
type detailEntry struct {
	Module  string       `json:"module"`
	Message string       `json:"message"`
	Fields  []enumReason `json:"fields"`
}

// enumReason is one enum field: reasons are the values the server sends,
// unsent are the other values of the enum, which it never sends here.
type enumReason struct {
	Field   string   `json:"field"`
	Enum    string   `json:"enum"`
	Reasons []string `json:"reasons"`
	Unsent  []string `json:"unsent"`
}

// bodyEntry is a reason in the JSON body of an HTTP route's error.
type bodyEntry struct {
	Module string `json:"module"`
	Route  string `json:"route"`
	Reason string `json:"reason"`
	Code   string `json:"code"`
}

// violationEntry is a reason of a table-content violation, a string.
type violationEntry struct {
	Module string `json:"module"`
	Reason string `json:"reason"`
}

// redirectEntry is a reason in the query of a redirect to an error page of the
// app (`/invite/error?reason=`).
type redirectEntry struct {
	Module string `json:"module"`
	Route  string `json:"route"`
	Reason string `json:"reason"`
}

func loadReasons(t *testing.T) reasonList {
	t.Helper()
	raw, err := os.ReadFile(reasonsFile)
	if err != nil {
		t.Fatalf("the closed list of error reasons is missing: %v", err)
	}
	var list reasonList
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("%s: %v", reasonsFile, err)
	}
	return list
}

// TestErrorReasonsListMatchesProto fails when an enum a detail carries has a
// value the list does not name (the server can send a reason the web has no
// sentence for), or the list names a value the enum does not have.
func TestErrorReasonsListMatchesProto(t *testing.T) {
	t.Parallel()
	for _, d := range loadReasons(t).Details {
		desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(d.Message))
		if err != nil {
			t.Errorf("%s is in the list but not in the protos: %v", d.Message, err)
			continue
		}
		md, ok := desc.(protoreflect.MessageDescriptor)
		if !ok {
			t.Errorf("%s is not a message", d.Message)
			continue
		}
		for _, f := range d.Fields {
			checkEnumField(t, md, f)
		}
	}
}

func checkEnumField(t *testing.T, md protoreflect.MessageDescriptor, f enumReason) {
	t.Helper()
	fd := md.Fields().ByName(protoreflect.Name(f.Field))
	if fd == nil || fd.Enum() == nil || string(fd.Enum().Name()) != f.Enum {
		t.Errorf("%s.%s is not a field of enum %s", md.FullName(), f.Field, f.Enum)
		return
	}
	values := fd.Enum().Values()
	prefix := strings.TrimSuffix(string(values.Get(0).Name()), "UNSPECIFIED")
	var want []string
	for i := 1; i < values.Len(); i++ {
		want = append(want, strings.TrimPrefix(string(values.Get(i).Name()), prefix))
	}
	got := slices.Concat(f.Reasons, f.Unsent)
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		t.Errorf("%s.%s: the list (reasons and unsent) differs from enum %s\n  enum only: %v\n  list only: %v",
			md.FullName(), f.Field, f.Enum, difference(want, got), difference(got, want))
	}
}

// difference is the values of a that b does not have.
func difference(a, b []string) []string {
	var out []string
	for _, v := range a {
		if !slices.Contains(b, v) {
			out = append(out, v)
		}
	}
	return out
}

// TestErrorReasonsAreSentByTheServer fails when a listed reason is an enum
// value no non-test Go file mentions: nobody sends it, so its sentence is
// dead weight and the value belongs in "unsent".
func TestErrorReasonsAreSentByTheServer(t *testing.T) {
	t.Parallel()
	used := map[string]bool{}
	for _, f := range parseSources(t) {
		ast.Inspect(f.file, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				used[sel.Sel.Name] = true
			}
			return true
		})
	}
	for _, d := range loadReasons(t).Details {
		for _, f := range d.Fields {
			for _, r := range f.Reasons {
				if !mentioned(used, d.Message, f, r) {
					t.Errorf("%s.%s: %s is listed as sent, but no Go file uses it; move it to \"unsent\"", d.Message, f.Field, r)
				}
			}
		}
	}
}

// mentioned says whether some Go file spells the generated constant of the
// enum value (the enum's Go name, an underscore and the proto value name).
func mentioned(used map[string]bool, message string, f enumReason, reason string) bool {
	desc, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(message))
	if err != nil {
		return true // TestErrorReasonsListMatchesProto reports it
	}
	fd := desc.(protoreflect.MessageDescriptor).Fields().ByName(protoreflect.Name(f.Field))
	if fd == nil || fd.Enum() == nil {
		return true
	}
	prefix := strings.TrimSuffix(string(fd.Enum().Values().Get(0).Name()), "UNSPECIFIED")
	return used[f.Enum+"_"+prefix+reason]
}

// TestErrorReasonsBodyMatchesRegistry fails when the Go registry of body
// reasons (platform/rpcerr/reasons.go) and the list disagree.
func TestErrorReasonsBodyMatchesRegistry(t *testing.T) {
	t.Parallel()
	registry := registryReasons(t)
	var listed []string
	for _, b := range loadReasons(t).Body {
		if !slices.Contains(listed, b.Reason) {
			listed = append(listed, b.Reason)
		}
	}
	slices.Sort(listed)
	got := slices.Sorted(func(yield func(string) bool) {
		for _, v := range registry {
			if !yield(v) {
				return
			}
		}
	})
	if !slices.Equal(listed, got) {
		t.Errorf("rpcerr.Reason constants and the list's body reasons differ\n  registry only: %v\n  list only: %v",
			difference(got, listed), difference(listed, got))
	}
}

// registryReasons is the constants of type Reason in platform/rpcerr/reasons.go,
// by name.
func registryReasons(t *testing.T) map[string]string {
	t.Helper()
	path := filepath.Join(backendRoot, "internal", "platform", "rpcerr", "reasons.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("the registry of body reasons is missing: %v", err)
	}
	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "Reason" {
				continue
			}
			for i, name := range vs.Names {
				if lit, ok := vs.Values[i].(*ast.BasicLit); ok {
					value, _ := strconv.Unquote(lit.Value)
					out[name.Name] = value
				}
			}
		}
	}
	return out
}

// TestErrorReasonsStringReasonsMatchConstants fails when the string reasons
// that come from named constants (the table-content violations: rules.Reason*
// and the characters package's reason*; the redirect to /invite/error:
// campaigns.InviteFailure*) and the list disagree.
func TestErrorReasonsStringReasonsMatchConstants(t *testing.T) {
	t.Parallel()
	list := loadReasons(t)
	var violations, redirects []string
	for _, v := range list.Violations {
		violations = append(violations, v.Reason)
	}
	for _, r := range list.Redirects {
		redirects = append(redirects, r.Reason)
	}
	files := parseSources(t)
	checks := []struct {
		name   string
		listed []string
		dirs   map[string]string // directory under internal/ -> constant name prefix
	}{
		{"violation", violations, map[string]string{"rules": "Reason", "characters": "reason"}},
		{"redirect", redirects, map[string]string{"campaigns": "InviteFailure"}},
	}
	for _, c := range checks {
		code := constValues(files, c.dirs)
		slices.Sort(code)
		slices.Sort(c.listed)
		if !slices.Equal(code, c.listed) {
			t.Errorf("the %s reasons in code and in the list differ\n  code only: %v\n  list only: %v",
				c.name, difference(code, c.listed), difference(c.listed, code))
		}
	}
}

// constValues is the values of the string constants whose name is the prefix
// of its directory followed by a capital letter.
func constValues(files []srcFile, dirs map[string]string) []string {
	var out []string
	for _, f := range files {
		prefix := dirs[filepath.ToSlash(filepath.Dir(f.rel))]
		if prefix == "" {
			continue
		}
		for name, value := range stringConsts(f.file) {
			rest, ok := strings.CutPrefix(name, prefix)
			if ok && rest != "" && rest[0] >= 'A' && rest[0] <= 'Z' {
				out = append(out, value)
			}
		}
	}
	return out
}

// stringConsts is the package-level string constants of a file, by name.
func stringConsts(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					break
				}
				if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					value, _ := strconv.Unquote(lit.Value)
					out[name.Name] = value
				}
			}
		}
	}
	return out
}

// TestErrorReasonsListIsClean fails on a duplicate entry or an empty word.
func TestErrorReasonsListIsClean(t *testing.T) {
	t.Parallel()
	list := loadReasons(t)
	seen := map[string]bool{}
	add := func(key string) {
		if key == "" || seen[key] {
			t.Errorf("empty or repeated entry %q", key)
		}
		seen[key] = true
	}
	for _, d := range list.Details {
		add("detail " + d.Message)
	}
	for _, b := range list.Body {
		add("body " + b.Route + " " + b.Reason)
		if b.Module == "" || b.Code == "" {
			t.Errorf("body reason %s of %q needs a module and a code", b.Reason, b.Route)
		}
	}
	for _, v := range list.Violations {
		add("violation " + v.Module + " " + v.Reason)
	}
}
