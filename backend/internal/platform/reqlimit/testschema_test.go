package reqlimit

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	optionsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/options/v1"
)

// The test schema, built with descriptors so the options can be set without a
// generated package of its own:
//
//	message Leaf { string text = 1; bytes blob = 2; repeated string tags = 3; string long_note = 4 [max_len = 20000]; }
//	message Node { string name = 1; repeated Node children = 2; }
//	message Req {
//	  repeated string names = 1;
//	  repeated Leaf leaves = 2;
//	  map<string, Leaf> by_key = 3;
//	  map<string, string> labels = 4;
//	  oneof pick { Leaf chosen = 5; string word = 6; }
//	  repeated string few = 7 [max_items = 5, max_len = 3];
//	  repeated string many = 8 [max_items = 1000];
//	  Node tree = 9;
//	  int32 n = 10;
//	  repeated int32 nums = 11;
//	}
type schema struct {
	leaf, node, req protoreflect.MessageDescriptor
}

func field(name string, num int32, typ descriptorpb.FieldDescriptorProto_Type, typeName string, repeated bool, opts ...func(*descriptorpb.FieldOptions)) *descriptorpb.FieldDescriptorProto {
	label := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	if repeated {
		label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	}
	f := &descriptorpb.FieldDescriptorProto{Name: proto.String(name), Number: proto.Int32(num), Type: typ.Enum(), Label: label.Enum()}
	if typeName != "" {
		f.TypeName = proto.String(typeName)
	}
	if len(opts) > 0 {
		f.Options = &descriptorpb.FieldOptions{}
		for _, o := range opts {
			o(f.Options)
		}
	}
	return f
}

func maxLen(n uint32) func(*descriptorpb.FieldOptions) {
	return func(o *descriptorpb.FieldOptions) { proto.SetExtension(o, optionsv1.E_MaxLen, n) }
}

func maxItems(n uint32) func(*descriptorpb.FieldOptions) {
	return func(o *descriptorpb.FieldOptions) { proto.SetExtension(o, optionsv1.E_MaxItems, n) }
}

func mapEntry(name string, key, val descriptorpb.FieldDescriptorProto_Type, valType string) *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: proto.String(name),
		Field: []*descriptorpb.FieldDescriptorProto{
			field("key", 1, key, "", false),
			field("value", 2, val, valType, false),
		},
		Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
	}
}

func newSchema(t *testing.T) schema {
	t.Helper()
	const (
		str  = descriptorpb.FieldDescriptorProto_TYPE_STRING
		byts = descriptorpb.FieldDescriptorProto_TYPE_BYTES
		msg  = descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
		i32  = descriptorpb.FieldDescriptorProto_TYPE_INT32
	)
	chosen := field("chosen", 5, msg, ".reqlimit.test.v1.Leaf", false)
	chosen.OneofIndex = proto.Int32(0)
	word := field("word", 6, str, "", false)
	word.OneofIndex = proto.Int32(0)
	fd, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:       proto.String("reqlimit/test/v1/test.proto"),
		Package:    proto.String("reqlimit.test.v1"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"meurpg/options/v1/limits.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("Leaf"), Field: []*descriptorpb.FieldDescriptorProto{
				field("text", 1, str, "", false),
				field("blob", 2, byts, "", false),
				field("tags", 3, str, "", true),
				field("long_note", 4, str, "", false, maxLen(20000)),
			}},
			{Name: proto.String("Node"), Field: []*descriptorpb.FieldDescriptorProto{
				field("name", 1, str, "", false),
				field("children", 2, msg, ".reqlimit.test.v1.Node", true),
			}},
			{
				Name: proto.String("Req"),
				Field: []*descriptorpb.FieldDescriptorProto{
					field("names", 1, str, "", true),
					field("leaves", 2, msg, ".reqlimit.test.v1.Leaf", true),
					field("by_key", 3, msg, ".reqlimit.test.v1.Req.ByKeyEntry", true),
					field("labels", 4, msg, ".reqlimit.test.v1.Req.LabelsEntry", true),
					chosen,
					word,
					field("few", 7, str, "", true, maxItems(5), maxLen(3)),
					field("many", 8, str, "", true, maxItems(1000)),
					field("tree", 9, msg, ".reqlimit.test.v1.Node", false),
					field("n", 10, i32, "", false),
					field("nums", 11, i32, "", true),
				},
				NestedType: []*descriptorpb.DescriptorProto{
					mapEntry("ByKeyEntry", str, msg, ".reqlimit.test.v1.Leaf"),
					mapEntry("LabelsEntry", str, str, ""),
				},
				OneofDecl: []*descriptorpb.OneofDescriptorProto{{Name: proto.String("pick")}},
			},
		},
	}, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatal(err)
	}
	return schema{
		leaf: fd.Messages().ByName("Leaf"),
		node: fd.Messages().ByName("Node"),
		req:  fd.Messages().ByName("Req"),
	}
}

func (s schema) fieldOf(md protoreflect.MessageDescriptor, name string) protoreflect.FieldDescriptor {
	return md.Fields().ByName(protoreflect.Name(name))
}

// leafWith builds a Leaf with the given text.
func (s schema) leafWith(text string) *dynamicpb.Message {
	m := dynamicpb.NewMessage(s.leaf)
	m.Set(s.fieldOf(s.leaf, "text"), protoreflect.ValueOfString(text))
	return m
}

func (s schema) newReq() *dynamicpb.Message { return dynamicpb.NewMessage(s.req) }

func (s schema) setStrings(m *dynamicpb.Message, name string, n int, each string) {
	list := m.Mutable(s.fieldOf(m.Descriptor(), name)).List()
	for range n {
		list.Append(protoreflect.ValueOfString(each))
	}
}

func (s schema) setLeaves(m *dynamicpb.Message, n int, text string) {
	list := m.Mutable(s.fieldOf(s.req, "leaves")).List()
	for range n {
		list.Append(protoreflect.ValueOfMessage(s.leafWith(text)))
	}
}

func text(n int) string { return strings.Repeat("a", n) }

func (s schema) set(m *dynamicpb.Message, name, value string) {
	m.Set(s.fieldOf(m.Descriptor(), name), protoreflect.ValueOfString(value))
}

// setBlob puts a leaf with n bytes in blob in the request's leaves.
func (s schema) setBlob(m *dynamicpb.Message, n int) {
	leaf := dynamicpb.NewMessage(s.leaf)
	leaf.Set(s.fieldOf(s.leaf, "blob"), protoreflect.ValueOfBytes(make([]byte, n)))
	m.Mutable(s.fieldOf(s.req, "leaves")).List().Append(protoreflect.ValueOfMessage(leaf))
}

// setLabels puts n entries in labels; the first has the given key and value.
func (s schema) setLabels(m *dynamicpb.Message, n int, key, value string) {
	labels := m.Mutable(s.fieldOf(s.req, "labels")).Map()
	labels.Set(protoreflect.ValueOfString(key).MapKey(), protoreflect.ValueOfString(value))
	for i := 1; i < n; i++ {
		labels.Set(protoreflect.ValueOfString("k"+text(i)).MapKey(), protoreflect.ValueOfString("v"))
	}
}

func (s schema) setNums(m *dynamicpb.Message, n int) {
	list := m.Mutable(s.fieldOf(s.req, "nums")).List()
	for i := range n {
		list.Append(protoreflect.ValueOfInt32(int32(i)))
	}
}

// setTree builds a chain of depth nodes under tree; the deepest has the name.
func (s schema) setTree(m *dynamicpb.Message, depth int, name string) {
	root := dynamicpb.NewMessage(s.node)
	cur := root
	for range depth {
		child := dynamicpb.NewMessage(s.node)
		cur.Mutable(s.fieldOf(s.node, "children")).List().Append(protoreflect.ValueOfMessage(child))
		cur = child
	}
	cur.Set(s.fieldOf(s.node, "name"), protoreflect.ValueOfString(name))
	m.Set(s.fieldOf(s.req, "tree"), protoreflect.ValueOfMessage(root))
}
