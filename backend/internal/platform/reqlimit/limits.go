// Package reqlimit holds the request size limits of the API
// (docs/architecture/api.md#request-size-limits).
//
// Two limits apply to every RPC:
//
//   - the fields of a decoded request: a repeated or map field has at most so
//     many items, a string or bytes field at most so many bytes. A field has a
//     default for its kind and may change it with the options of
//     proto/meurpg/options/v1/limits.proto (Interceptor enforces it);
//   - the body of a unary request as it is read, per procedure (Middleware).
package reqlimit

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	optionsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/options/v1"
)

// The default limit of a field that has no option, by kind of field.
const (
	// DefaultMaxItems is the most items of a repeated field, or entries of a map.
	DefaultMaxItems = 100
	// DefaultMaxString is the most bytes of a string (of each element of a
	// repeated string, of each key or value of a map).
	DefaultMaxString = 8 << 10
	// DefaultMaxBytes is the most bytes of a bytes field, counted the same way.
	DefaultMaxBytes = 4 << 10
)

// ItemsLimit is the most items a repeated field, or entries a map field, accepts:
// its max_items option if set, else DefaultMaxItems. It is 0 for any other field.
func ItemsLimit(fd protoreflect.FieldDescriptor) uint32 {
	if !fd.IsList() && !fd.IsMap() {
		return 0
	}
	if n := option(fd, optionsv1.E_MaxItems); n > 0 {
		return n
	}
	return DefaultMaxItems
}

// LenLimit is the most bytes a value of the given kind accepts in the field:
// the field's max_len option if set, else the default for the kind. The kind
// is the field's own (the element of a list) or, in a map, the key's or the
// value's. It is 0 for a kind that is neither string nor bytes.
func LenLimit(fd protoreflect.FieldDescriptor, kind protoreflect.Kind) uint32 {
	def := defaultLen(kind)
	if def == 0 {
		return 0
	}
	if n := option(fd, optionsv1.E_MaxLen); n > 0 {
		return n
	}
	return def
}

// EffectiveLimit is the limit that bounds a field: the item count of a
// repeated or map field, else the byte length of a string or bytes field. It
// is 0 only for a field with nothing to bound (a number, a bool, an enum, a
// message), never for a field that has something to bound. A list of strings
// has two limits; this one is the count, and LenLimit gives the other.
func EffectiveLimit(fd protoreflect.FieldDescriptor) uint32 {
	if n := ItemsLimit(fd); n > 0 {
		return n
	}
	return LenLimit(fd, fd.Kind())
}

// Raised reports whether the field's options let it go over the defaults of
// its kind.
func Raised(fd protoreflect.FieldDescriptor) bool {
	if ItemsLimit(fd) > DefaultMaxItems {
		return true
	}
	for _, k := range valueKinds(fd) {
		if def := defaultLen(k); def > 0 && LenLimit(fd, k) > def {
			return true
		}
	}
	return false
}

// defaultLen is the default byte limit of a kind, 0 for a kind with none.
func defaultLen(k protoreflect.Kind) uint32 {
	switch k {
	case protoreflect.StringKind:
		return DefaultMaxString
	case protoreflect.BytesKind:
		return DefaultMaxBytes
	}
	return 0
}

// valueKinds are the kinds of the values of a field that a max_len bounds.
func valueKinds(fd protoreflect.FieldDescriptor) []protoreflect.Kind {
	if fd.IsMap() {
		return []protoreflect.Kind{fd.MapKey().Kind(), fd.MapValue().Kind()}
	}
	return []protoreflect.Kind{fd.Kind()}
}

func option(fd protoreflect.FieldDescriptor, xt protoreflect.ExtensionType) uint32 {
	opts, ok := fd.Options().(*descriptorpb.FieldOptions)
	if !ok || opts == nil {
		return 0
	}
	n, _ := proto.GetExtension(opts, xt).(uint32)
	return n
}
