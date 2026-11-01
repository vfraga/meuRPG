package reqlimit

import (
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	optionsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/options/v1"

	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaignpackage/v1"
	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/identity/v1"
	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/notes/v1"
	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	_ "github.com/PuraFome/meuRPG/backend/gen/meurpg/system/v1"
)

// procedure is one RPC of the API.
type procedure struct {
	name  string // "/meurpg.x.v1.Service/Method"
	input protoreflect.MessageDescriptor
}

func procedures(t *testing.T) []procedure {
	t.Helper()
	var out []procedure
	protoregistry.GlobalFiles.RangeFiles(func(f protoreflect.FileDescriptor) bool {
		if !strings.HasPrefix(string(f.Package()), "meurpg.") {
			return true
		}
		for i := range f.Services().Len() {
			svc := f.Services().Get(i)
			for j := range svc.Methods().Len() {
				m := svc.Methods().Get(j)
				out = append(out, procedure{"/" + string(svc.FullName()) + "/" + string(m.Name()), m.Input()})
			}
		}
		return true
	})
	if len(out) < 100 {
		t.Fatalf("found only %d procedures: the services are not registered", len(out))
	}
	return out
}

// reachableFields calls visit for every field of md and of the messages it
// reaches (message fields, list elements, map values), each message once.
// It follows descriptors, so the visited set is what ends a recursive type.
func reachableFields(md protoreflect.MessageDescriptor, visit func(protoreflect.FieldDescriptor)) {
	seen := map[protoreflect.FullName]bool{}
	var walk func(protoreflect.MessageDescriptor)
	walk = func(md protoreflect.MessageDescriptor) {
		if seen[md.FullName()] {
			return
		}
		seen[md.FullName()] = true
		for i := range md.Fields().Len() {
			fd := md.Fields().Get(i)
			visit(fd)
			switch {
			case fd.IsMap():
				if v := fd.MapValue(); v.Message() != nil {
					walk(v.Message())
				}
			case fd.Message() != nil:
				walk(fd.Message())
			}
		}
	}
	walk(md)
}

// The defaults, spelled out here on purpose: the test compares the package's
// answers with these, so a constant changed to 0 (or silently) fails it.
const (
	wantItems  = 100
	wantString = 8 << 10
	wantBytes  = 4 << 10
	wantRead   = 64 << 10
	wantBack   = 4 << 20
)

func TestDefaultsAreNotZero(t *testing.T) {
	for name, got := range map[string][2]int64{
		"DefaultMaxItems":  {DefaultMaxItems, wantItems},
		"DefaultMaxString": {DefaultMaxString, wantString},
		"DefaultMaxBytes":  {DefaultMaxBytes, wantBytes},
		"DefaultReadLimit": {DefaultReadLimit, wantRead},
		"Backstop":         {Backstop, wantBack},
	} {
		if got[0] <= 0 || got[0] != got[1] {
			t.Errorf("%s = %d, want %d", name, got[0], got[1])
		}
	}
}

// expected is the limit of a value of a kind that the test works out on its
// own: the field's option if it has one, else the default of the kind.
func expected(fd protoreflect.FieldDescriptor, xt protoreflect.ExtensionType, def uint32) uint32 {
	if opts, ok := fd.Options().(*descriptorpb.FieldOptions); ok && opts != nil && proto.HasExtension(opts, xt) {
		if n, _ := proto.GetExtension(opts, xt).(uint32); n > 0 {
			return n
		}
	}
	return def
}

func expectedLen(fd protoreflect.FieldDescriptor, k protoreflect.Kind) uint32 {
	switch k {
	case protoreflect.StringKind:
		return expected(fd, optionsv1.E_MaxLen, wantString)
	case protoreflect.BytesKind:
		return expected(fd, optionsv1.E_MaxLen, wantBytes)
	}
	return 0
}

// TestRegistry checks the whole API against the limits, with no database.
// For every procedure of every meurpg.* service:
//
//   - its read limit is the default or its raisedReadLimits entry, and an
//     entry is over the default and at most the backstop;
//   - every field its request reaches (nested messages, list elements and map
//     values included) has the limit the test works out itself, option or
//     default of the kind, and that limit is not 0: no list, map, string or
//     bytes field is unbounded;
//   - a field whose limit is over the default of its kind sits only on
//     procedures that have a raisedReadLimits entry. The test does not add up
//     a worst case: whoever writes the option chooses the procedure's read
//     limit on purpose, and the test makes sure they did.
//
// Every entry of raisedReadLimits names a real procedure.
func TestRegistry(t *testing.T) {
	procs := procedures(t)
	raisedMu.RLock()
	raised := maps.Clone(raisedReadLimits)
	raisedMu.RUnlock()
	known := map[string]bool{}
	var lists, texts int
	for _, p := range procs {
		known[p.name] = true
		entry, isRaised := raised[p.name]
		want := int64(wantRead)
		if isRaised {
			want = entry
		}
		if got := ReadLimit(p.name); got != want || got <= 0 || got > wantBack {
			t.Errorf("%s: read limit %d, want %d (the default, or its entry) within 1..%d", p.name, got, want, wantBack)
		}
		reachableFields(p.input, func(fd protoreflect.FieldDescriptor) {
			l, x := checkBounded(t, p.name, fd)
			lists, texts = lists+l, texts+x
			if raisedByOption(fd) && !isRaised {
				t.Errorf("%s reaches %s, which raises its limit over the defaults, but the procedure has no entry in raisedReadLimits", p.name, fd.FullName())
			}
		})
	}
	for name, n := range raised {
		switch {
		case !known[name]:
			t.Errorf("raisedReadLimits names %s, which is not a procedure", name)
		case n <= wantRead:
			t.Errorf("raisedReadLimits[%s] = %d does not raise the default %d", name, n, wantRead)
		case n > wantBack:
			t.Errorf("raisedReadLimits[%s] = %d is over the backstop %d", name, n, wantBack)
		}
	}
	// The checks above only mean something if the walk saw what a request holds.
	if lists < 50 || texts < 100 {
		t.Errorf("the walk found only %d lists or maps and %d strings or bytes fields", lists, texts)
	}
}

// raisedByOption says, from the test's own numbers, whether a field's option
// puts its limit over the default of its kind.
func raisedByOption(fd protoreflect.FieldDescriptor) bool {
	if (fd.IsList() || fd.IsMap()) && expected(fd, optionsv1.E_MaxItems, wantItems) > wantItems {
		return true
	}
	kinds := []protoreflect.Kind{fd.Kind()}
	if fd.IsMap() {
		kinds = []protoreflect.Kind{fd.MapKey().Kind(), fd.MapValue().Kind()}
	}
	for _, k := range kinds {
		def := map[protoreflect.Kind]uint32{protoreflect.StringKind: wantString, protoreflect.BytesKind: wantBytes}[k]
		if def > 0 && expectedLen(fd, k) > def {
			return true
		}
	}
	return false
}

// checkBounded compares the package's limits of a field with the test's own,
// and returns how many list/map and string/bytes limits it checked.
func checkBounded(t *testing.T, procedure string, fd protoreflect.FieldDescriptor) (lists, texts int) {
	t.Helper()
	if fd.IsList() || fd.IsMap() {
		lists++
		want := expected(fd, optionsv1.E_MaxItems, wantItems)
		if got := ItemsLimit(fd); got != want || got == 0 {
			t.Errorf("%s: %s has item limit %d, want %d", procedure, fd.FullName(), got, want)
		}
		if got := EffectiveLimit(fd); got == 0 {
			t.Errorf("%s: %s has no effective limit", procedure, fd.FullName())
		}
	}
	kinds := []protoreflect.Kind{fd.Kind()}
	if fd.IsMap() {
		kinds = []protoreflect.Kind{fd.MapKey().Kind(), fd.MapValue().Kind()}
	}
	for _, k := range kinds {
		want := expectedLen(fd, k)
		if got := LenLimit(fd, k); got != want {
			t.Errorf("%s: %s has length limit %d for %v, want %d", procedure, fd.FullName(), got, k, want)
		}
		if want > 0 {
			texts++
		}
	}
	if !fd.IsList() && !fd.IsMap() && expectedLen(fd, fd.Kind()) > 0 {
		if want, got := expectedLen(fd, fd.Kind()), EffectiveLimit(fd); got != want || got == 0 {
			t.Errorf("%s: %s has effective limit %d, want %d", procedure, fd.FullName(), got, want)
		}
	}
	return lists, texts
}

// TestMiddlewareEnforcesTheReadLimitOfEveryProcedure runs the production
// middleware over a handler that reads the whole body, at the path of every
// procedure: a body of the procedure's limit (the default, or its entry in
// raisedReadLimits as the test reads it) is read, and one byte more is refused.
// It cannot pass for a procedure whose limit is not enforced.
func TestMiddlewareEnforcesTheReadLimitOfEveryProcedure(t *testing.T) {
	raisedMu.RLock()
	raised := maps.Clone(raisedReadLimits)
	raisedMu.RUnlock()
	read := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		}
	}))
	send := func(path string, n int64) int {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, io.LimitReader(zeros{}, n))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		read.ServeHTTP(rec, req)
		return rec.Code
	}
	for _, p := range procedures(t) {
		limit := int64(wantRead)
		if n, ok := raised[p.name]; ok {
			limit = n
		}
		if code := send(p.name, limit); code != http.StatusOK {
			t.Errorf("%s: a body of %d bytes got %d, want it read", p.name, limit, code)
		}
		if code := send(p.name, limit+1); code != http.StatusRequestEntityTooLarge {
			t.Errorf("%s: a body of %d bytes got %d, want it refused", p.name, limit+1, code)
		}
	}
}

// zeros is an endless reader of zero bytes.
type zeros struct{}

func (zeros) Read(p []byte) (int, error) { clear(p); return len(p), nil }
