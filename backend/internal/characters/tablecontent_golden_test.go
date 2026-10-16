package characters

import (
	"bytes"
	"encoding/json"
	"errors"
	stdflag "flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// The golden files of the table content's characterization (testdata/table_content_*.golden.json):
// what each viewer reads of the table's entries, what the switches answer and what every
// refusal says. Run with -update-table-content to write them again after a change of
// behavior that was meant.

var updateTableContent = stdflag.Bool("update-table-content", false, "write the golden files of the table content again")

// tableContentGoldenTimes are the JSON names of the times the golden files leave out: they
// come from the test's clock.
var tableContentGoldenTimes = map[string]bool{"createdAt": true, "updatedAt": true, "archivedAt": true}

// tableContentGoldenStep is one labeled value of a golden file: a message, an error or a
// plain value.
type tableContentGoldenStep struct {
	label string
	value any
}

// tableContentJSON is the stable JSON of a value: a message through protojson (times
// masked), an error as its shape, anything else as encoding/json makes it.
func tableContentJSON(t *testing.T, v any) any {
	t.Helper()
	switch m := v.(type) {
	case error:
		return tableRefusalShape(m)
	case proto.Message:
		raw, err := protojson.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		var tree any
		if err := json.Unmarshal(raw, &tree); err != nil {
			t.Fatal(err)
		}
		return maskTableContentTimes(tree)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	return tree
}

// maskTableContentTimes replaces every time of the tree by "<time>", keeping that it is set.
func maskTableContentTimes(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if tableContentGoldenTimes[k] {
				x[k] = "<time>"
				continue
			}
			x[k] = maskTableContentTimes(child)
		}
	case []any:
		for i, child := range x {
			x[i] = maskTableContentTimes(child)
		}
	}
	return v
}

// tableContentGolden compares the steps with testdata/table_content_<name>.golden.json, the
// strings of replace swapped first (ids by names).
func tableContentGolden(t *testing.T, name string, replace map[string]string, steps ...tableContentGoldenStep) {
	t.Helper()
	var doc []map[string]any
	for _, s := range steps {
		doc = append(doc, map[string]any{"step": s.label, "value": tableContentJSON(t, s.value)})
	}
	got, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	// The longest first, so an id inside another is never cut.
	olds := make([]string, 0, len(replace))
	for old := range replace {
		olds = append(olds, old)
	}
	sort.Slice(olds, func(i, j int) bool { return len(olds[i]) > len(olds[j]) })
	for _, old := range olds {
		got = bytes.ReplaceAll(got, []byte(old), []byte(replace[old]))
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "table_content_"+name+".golden.json")
	if *updateTableContent {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the golden file: %v (write it with -update-table-content)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from %s:\n--- got\n%s\n--- want\n%s", name, path, got, want)
	}
}

// tableRefusalShape is everything a refusal tells the web: its code, its message and each
// detail (the violations with field, reason, key and message; the blocked reason and key;
// the invalid field).
func tableRefusalShape(err error) string {
	if err == nil {
		return "<nil>"
	}
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok {
		return "not a connect error: " + err.Error()
	}
	parts := []string{ce.Code().String() + ": " + ce.Message()}
	for _, d := range ce.Details() {
		v, derr := d.Value()
		if derr != nil {
			parts = append(parts, "undecodable detail "+d.Type())
			continue
		}
		switch m := v.(type) {
		case *rulesv1.TableContentRefusal:
			for _, x := range m.GetViolations() {
				parts = append(parts, fmt.Sprintf("violation field=%q reason=%q key=%q message=%q", x.GetField(), x.GetReason(), x.GetKey(), x.GetMessage()))
			}
		case *rulesv1.TableContentBlocked:
			parts = append(parts, fmt.Sprintf("blocked reason=%s key=%q", m.GetReason(), m.GetKey()))
		case *charactersv1.InvalidField:
			parts = append(parts, fmt.Sprintf("invalid field=%q", m.GetField()))
		default:
			parts = append(parts, fmt.Sprintf("detail %T", v))
		}
	}
	return strings.Join(parts, " | ")
}
