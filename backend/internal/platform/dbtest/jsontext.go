package dbtest

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
)

// SortJSONKeys rewrites every JSON object or array in s with the keys of each
// object in byte order, in the layout both databases print jsonb in
// ({"a": 1, "b": [1, 2]}). A test that compares a jsonb column read as text, or
// a row built from one, calls it on both sides' text: CockroachDB prints the
// keys in byte order, PostgreSQL by length first ({"to": 1, "from": 2}), so the
// same value has two texts. Text that is not JSON (a SQL array {1,2}, a name),
// and JSON in a layout other than jsonb's, is left as it is.
func SortJSONKeys(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '{' && s[i] != '[' {
			out.WriteByte(s[i])
			i++
			continue
		}
		dec := json.NewDecoder(strings.NewReader(s[i:]))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			out.WriteByte(s[i])
			i++
			continue
		}
		n := int(dec.InputOffset())
		var sorted strings.Builder
		render(&sorted, v)
		// Both databases print jsonb in one layout, so only the order of the keys differs, and the length with it
		// stays. Another length is text this function does not know (compact or indented JSON, other escapes): it
		// stays as it is, so a difference in it fails the test instead of being hidden.
		if sorted.Len() == n {
			out.WriteString(sorted.String())
		} else {
			out.WriteString(s[i : i+n])
		}
		i += n
	}
	return out.String()
}

func render(out *strings.Builder, v any) {
	switch v := v.(type) {
	case map[string]any:
		renderObject(out, v)
	case []any:
		renderArray(out, v)
	case json.Number:
		out.WriteString(v.String())
	case string:
		out.WriteString(quote(v))
	case bool:
		out.WriteString(strconv.FormatBool(v))
	default:
		out.WriteString("null")
	}
}

func renderObject(out *strings.Builder, v map[string]any) {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(quote(k))
		out.WriteString(": ")
		render(out, v[k])
	}
	out.WriteByte('}')
}

func renderArray(out *strings.Builder, v []any) {
	out.WriteByte('[')
	for i, e := range v {
		if i > 0 {
			out.WriteString(", ")
		}
		render(out, e)
	}
	out.WriteByte(']')
}

func quote(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}
