package dbtest

import "testing"

func TestSortJSONKeys(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, in, want string }{
		{"keys by length become keys by bytes", `{"to": "Pensantus", "from": "Toren", "round": 1}`, `{"from": "Toren", "round": 1, "to": "Pensantus"}`},
		{"nested", `{"b": {"zz": [1, {"y": 2, "x": 3}], "a": null}, "a": true}`, `{"a": true, "b": {"a": null, "zz": [1, {"x": 3, "y": 2}]}}`},
		{"inside a row", `hp=7 slots={2,0,1} resources={"gone": 2, "arcane_recovery": 1} rev=4`, `hp=7 slots={2,0,1} resources={"arcane_recovery": 1, "gone": 2} rev=4`},
		{"an empty object and a SQL array stay", `by_die={} created={1,0,0}`, `by_die={} created={1,0,0}`},
		{"numbers and html keep their text", `{"b": 1.50, "a": "<&>"}`, `{"a": "<&>", "b": 1.50}`},
		{"compact json is not jsonb's layout and stays", `{"b":1,"a":2}`, `{"b":1,"a":2}`},
		{"not json", `plain [text] {not json`, `plain [text] {not json`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := SortJSONKeys(tt.in); got != tt.want {
				t.Errorf("SortJSONKeys(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
