package characters

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"

	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// reasonUnknownField is the violation of a field the message does not have: a pack written
// with a misspelled name would otherwise import with the field dropped.
const reasonUnknownField = "unknown_field"

// readPackJSON reads a pack file strictly. The Connect JSON codec drops the fields it does
// not know, so the file itself is read here: the pack is what protojson makes of it (which
// still ignores the unknown fields), and every unknown field is found by walking the JSON
// against the message. Unknown fields of an entry are returned by the entry's index, at the
// path of the field inside the entry ("table_feat.prerequisite.proficiency"); the ones
// outside any entry are violations of the pack ("pack.name2").
func readPackJSON(data []byte) (*rulesv1.TableContentPack, map[int][]*rulesv1.TableContentViolation, []*rulesv1.TableContentViolation) {
	if len(data) > MaxPackBytes {
		return nil, nil, []*rulesv1.TableContentViolation{packViolation("pack", reasonSizeLimit, "the pack is over %d bytes", MaxPackBytes)}
	}
	bad := func() (*rulesv1.TableContentPack, map[int][]*rulesv1.TableContentViolation, []*rulesv1.TableContentViolation) {
		return nil, nil, []*rulesv1.TableContentViolation{packViolation("pack", rules.ReasonValue, "the file is not a content pack in proto JSON")}
	}
	var tree any
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&tree); err != nil {
		return bad()
	}
	if _, isObject := tree.(map[string]any); !isObject {
		return bad()
	}
	pack := &rulesv1.TableContentPack{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, pack); err != nil {
		return bad()
	}
	unknown := map[int][]*rulesv1.TableContentViolation{}
	var top []*rulesv1.TableContentViolation
	walkUnknown(pack.ProtoReflect().Descriptor(), tree, "", func(path string) {
		if idx, rest, ok := entryPath(path); ok {
			key := ""
			if idx < len(pack.GetEntries()) {
				key = pack.GetEntries()[idx].GetKey()
			}
			unknown[idx] = append(unknown[idx], &rulesv1.TableContentViolation{
				Field: rest, Reason: reasonUnknownField, Key: key, Message: fmt.Sprintf("the field %q does not exist", lastName(path)),
			})
			return
		}
		top = append(top, packViolation("pack."+path, reasonUnknownField, "the field %q does not exist", lastName(path)))
	})
	return pack, unknown, top
}

// walkUnknown calls found with the path of every object key that the message does not have,
// recursing into message fields and lists of messages (the names may be the proto names or
// the JSON names, as protojson reads either). Paths use the proto names and list indexes.
func walkUnknown(md protoreflect.MessageDescriptor, v any, path string, found func(string)) {
	obj, ok := v.(map[string]any)
	if !ok {
		return
	}
	for name, child := range obj {
		fd := fieldNamed(md, name)
		if fd == nil {
			found(joinPath(path, name))
			continue
		}
		walkUnknownField(fd, child, joinPath(path, string(fd.Name())), found)
	}
}

// fieldNamed is the field of md with the proto name or the JSON name, or nil.
func fieldNamed(md protoreflect.MessageDescriptor, name string) protoreflect.FieldDescriptor {
	if fd := md.Fields().ByName(protoreflect.Name(name)); fd != nil {
		return fd
	}
	return md.Fields().ByJSONName(name)
}

// walkUnknownField walks the value of one known field: a message, or each message of a list.
// A scalar or a map holds no field of its own to check.
func walkUnknownField(fd protoreflect.FieldDescriptor, child any, here string, found func(string)) {
	if fd.Message() == nil || fd.IsMap() {
		return
	}
	if !fd.IsList() {
		walkUnknown(fd.Message(), child, here, found)
		return
	}
	items, _ := child.([]any)
	for i, item := range items {
		walkUnknown(fd.Message(), item, here+"["+strconv.Itoa(i)+"]", found)
	}
}

func joinPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

// entryPath splits "entries[3].table_feat.x" into 3 and "table_feat.x".
func entryPath(path string) (int, string, bool) {
	rest, ok := strings.CutPrefix(path, "entries[")
	if !ok {
		return 0, "", false
	}
	num, after, ok := strings.Cut(rest, "]")
	idx, err := strconv.Atoi(num)
	if !ok || err != nil {
		return 0, "", false
	}
	return idx, strings.TrimPrefix(after, "."), true
}

func lastName(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[i+1:]
	}
	return path
}
