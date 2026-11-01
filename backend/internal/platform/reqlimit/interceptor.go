package reqlimit

import (
	"context"
	"errors"
	"sync"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	systemv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/system/v1"
)

// Interceptor refuses, as `invalid_argument` with a RequestRefused detail, a
// request with a field over its limit. It runs after the request is decoded
// and before the handler, for unary calls and for the requests a stream receives.
// The client sees the field's path, not the limit.
func Interceptor() connect.Interceptor { return interceptor{} }

type interceptor struct{}

func (interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if !req.Spec().IsClient {
			if err := Check(req.Any()); err != nil {
				return nil, err
			}
		}
		return next(ctx, req)
	}
}

func (interceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (interceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return next(ctx, checkedConn{conn})
	}
}

// checkedConn checks every message the handler receives (a server stream has one).
type checkedConn struct{ connect.StreamingHandlerConn }

func (c checkedConn) Receive(msg any) error {
	if err := c.StreamingHandlerConn.Receive(msg); err != nil {
		return err
	}
	return Check(msg)
}

// Check returns the refusal for a message with a field over its limit, or nil.
func Check(msg any) error {
	m, ok := msg.(proto.Message)
	if !ok {
		return nil
	}
	v := firstViolation(m.ProtoReflect())
	if v == nil {
		return nil
	}
	err := connect.NewError(connect.CodeInvalidArgument, errors.New("request over its size limit"))
	if d, derr := connect.NewErrorDetail(&systemv1.RequestRefused{Reason: v.reason, Field: v.field}); derr == nil {
		err.AddDetail(d)
	}
	// Built before the interceptors a module mounts, so it says no-store itself.
	err.Meta().Set("Cache-Control", "no-store")
	return err
}

type violation struct {
	reason systemv1.RequestRefusedReason
	field  string // the path from the message that was walked, in proto field names
}

// firstViolation walks the set fields of m in declaration order (so the
// first violation is the same on every run), checking a list's or map's size
// before it looks inside. It follows the message in hand, never a descriptor,
// so a recursive type ends where the data does. Extension fields are not
// walked: the API's requests have none.
func firstViolation(m protoreflect.Message) *violation {
	fields := m.Descriptor().Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		switch fd.Kind() {
		case protoreflect.StringKind, protoreflect.BytesKind, protoreflect.MessageKind, protoreflect.GroupKind:
		default:
			if !fd.IsList() { // a number, a bool or an enum has nothing to bound
				continue
			}
		}
		if !m.Has(fd) {
			continue
		}
		if found := checkField(fd, m.Get(fd)); found != nil {
			if found.field == "" {
				found.field = string(fd.Name())
			} else {
				found.field = string(fd.Name()) + "." + found.field
			}
			return found
		}
	}
	return nil
}

// plan is the limits of a field, resolved once: reading the options of a
// descriptor is not free, and the descriptors are few.
type plan struct{ items, elem, key, val uint32 }

var plans sync.Map // protoreflect.FieldDescriptor -> *plan

func planOf(fd protoreflect.FieldDescriptor) *plan {
	if p, ok := plans.Load(fd); ok {
		if pl, ok := p.(*plan); ok {
			return pl
		}
	}
	p := &plan{items: ItemsLimit(fd)}
	if fd.IsMap() {
		p.key, p.val = LenLimit(fd, fd.MapKey().Kind()), LenLimit(fd, fd.MapValue().Kind())
	} else {
		p.elem = LenLimit(fd, fd.Kind())
	}
	if actual, loaded := plans.LoadOrStore(fd, p); loaded {
		if pl, ok := actual.(*plan); ok {
			return pl
		}
	}
	return p
}

// checkField checks one set field; the path of the violation is relative to it.
func checkField(fd protoreflect.FieldDescriptor, v protoreflect.Value) *violation {
	p := planOf(fd)
	switch {
	case fd.IsList():
		l := v.List()
		if tooMany(l.Len(), p.items) {
			return &violation{reason: systemv1.RequestRefusedReason_REQUEST_REFUSED_REASON_LIST_TOO_LONG}
		}
		for i := range l.Len() {
			if bad := checkValue(fd.Kind(), p.elem, l.Get(i)); bad != nil {
				return bad
			}
		}
	case fd.IsMap():
		mp := v.Map()
		if tooMany(mp.Len(), p.items) {
			return &violation{reason: systemv1.RequestRefusedReason_REQUEST_REFUSED_REASON_LIST_TOO_LONG}
		}
		var bad *violation
		mp.Range(func(k protoreflect.MapKey, mv protoreflect.Value) bool {
			if bad = checkValue(fd.MapKey().Kind(), p.key, k.Value()); bad == nil {
				bad = checkValue(fd.MapValue().Kind(), p.val, mv)
			}
			return bad == nil
		})
		return bad
	default:
		return checkValue(fd.Kind(), p.elem, v)
	}
	return nil
}

// checkValue checks one value of the given kind: a string or bytes against
// its byte limit, a message by walking into it.
func checkValue(kind protoreflect.Kind, limit uint32, v protoreflect.Value) *violation {
	switch kind {
	case protoreflect.StringKind:
		if len(v.String()) > int(limit) {
			return &violation{reason: systemv1.RequestRefusedReason_REQUEST_REFUSED_REASON_TEXT_TOO_LONG}
		}
	case protoreflect.BytesKind:
		if len(v.Bytes()) > int(limit) {
			return &violation{reason: systemv1.RequestRefusedReason_REQUEST_REFUSED_REASON_TEXT_TOO_LONG}
		}
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return firstViolation(v.Message())
	}
	return nil
}

// tooMany says a list or map of n entries is over its limit.
func tooMany(n int, limit uint32) bool { return int64(n) > int64(limit) }
