package play

import (
	"context"
	"sync"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// A roll that a feature changed names the feature in Portuguese next to its content key (Reroll.source_name,
// RerollQuestion.source_name and every treated_as_source_name), so the screen can say "Dado Sortudo da Mesa" for a table
// feature, not only for the SRD ones it knows. The name is added to the answer of an RPC, after every rule of who reads
// what has run (RN-10, RN-20): it follows the key and appears only where the key is already there, which is the roller's
// player and the master. An entry the master retired or switched off has no name (RN-23).

// sourceNamePair is a key field of a message type and the field that carries its name.
type sourceNamePair struct{ key, name protoreflect.Name }

// namedSourceFields is every message type that names a feature next to its content key. A test fails when a message of the
// protos has such a pair that is missing here, so a new field cannot go unnamed by accident.
var namedSourceFields = map[protoreflect.FullName][]sourceNamePair{
	"meurpg.play.v1.Reroll":                   {{"source_key", "source_name"}},
	"meurpg.play.v1.CountedDie":               {{"source_key", "source_name"}},
	"meurpg.play.v1.RerollQuestion":           {{"source_key", "source_name"}},
	"meurpg.play.v1.DiceRoll":                 {{"treated_as_source", "treated_as_source_name"}},
	"meurpg.play.v1.CheckRoll":                {{"treated_as_source", "treated_as_source_name"}},
	"meurpg.play.v1.EffectSaveResult":         {{"treated_as_source", "treated_as_source_name"}},
	"meurpg.play.v1.CombatLogHitRider":        {{"treated_as_source", "treated_as_source_name"}},
	"meurpg.play.v1.CombatLogEffect":          {{"treated_as_source", "treated_as_source_name"}},
	"meurpg.play.v1.ChannelDivinityTurn":      {{"treated_as_source", "treated_as_source_name"}},
	"meurpg.play.v1.LegendaryResistanceOffer": {{"treated_as_source", "treated_as_source_name"}},
	"meurpg.play.v1.Combatant":                {{"initiative_treated_as_source", "initiative_treated_as_source_name"}},
}

// canName says whether a response of this type can hold a message that names a source: the type itself or any message
// reachable through its fields. It is worked out once per type, so the walk skips the answers that cannot.
var canName sync.Map // protoreflect.FullName -> bool

func canNameSources(md protoreflect.MessageDescriptor) bool {
	if v, ok := canName.Load(md.FullName()); ok {
		if can, isBool := v.(bool); isBool {
			return can
		}
	}
	ok := reaches(md, map[protoreflect.FullName]bool{})
	canName.Store(md.FullName(), ok)
	return ok
}

func reaches(md protoreflect.MessageDescriptor, seen map[protoreflect.FullName]bool) bool {
	if _, ok := namedSourceFields[md.FullName()]; ok {
		return true
	}
	if seen[md.FullName()] {
		return false
	}
	seen[md.FullName()] = true
	fields := md.Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		if fd.IsMap() {
			fd = fd.MapValue()
		}
		if m := fd.Message(); m != nil && reaches(m, seen) {
			return true
		}
	}
	return false
}

// namedSource is a message of a response that carries a key to name.
type namedSource struct {
	msg   protoreflect.Message
	pairs []sourceNamePair
}

func collectSources(m protoreflect.Message, out *[]namedSource) {
	if !canNameSources(m.Descriptor()) {
		return
	}
	if ps := namedSourceFields[m.Descriptor().FullName()]; len(ps) > 0 {
		*out = append(*out, namedSource{msg: m, pairs: ps})
	}
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsMap():
			if fd.MapValue().Message() != nil {
				v.Map().Range(func(_ protoreflect.MapKey, mv protoreflect.Value) bool {
					collectSources(mv.Message(), out)
					return true
				})
			}
		case fd.Message() == nil:
		case fd.IsList():
			for i := range v.List().Len() {
				collectSources(v.List().Get(i).Message(), out)
			}
		default:
			collectSources(v.Message(), out)
		}
		return true
	})
}

// nameSources fills the names of the keys a response carries, with the names the campaign's content gives.
func (s *Service) nameSources(ctx context.Context, campaignID string, res proto.Message) {
	if campaignID == "" || !canNameSources(res.ProtoReflect().Descriptor()) {
		return
	}
	var found []namedSource
	collectSources(res.ProtoReflect(), &found)
	fieldOf := func(m protoreflect.Message, n protoreflect.Name) protoreflect.FieldDescriptor {
		return m.Descriptor().Fields().ByName(n)
	}
	var namer func(string) string
	for _, f := range found {
		for _, p := range f.pairs {
			key := f.msg.Get(fieldOf(f.msg, p.key)).String()
			if key == "" {
				continue
			}
			if namer == nil {
				// Read once, and only for an answer that holds a key. The handler's own content read is inside its closed
				// transaction and not handed on, so this asks the live source, which keeps the built content and reads at most
				// the campaign's revision.
				var err error
				if namer, err = s.roster.SourceNames(ctx, nil, campaignID); err != nil {
					s.logger.WarnContext(ctx, "play: cannot read the content names of the rolls", "error", err)
					return
				}
			}
			f.msg.Set(fieldOf(f.msg, p.name), protoreflect.ValueOfString(namer(key)))
		}
	}
}

// sourceNameInterceptor names the features of the rolls an answer carries.
func sourceNameInterceptor(s *Service) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			res, err := next(ctx, req)
			if err != nil || res == nil {
				return res, err
			}
			if msg, ok := res.Any().(proto.Message); ok {
				s.nameSources(ctx, campaignOf(req.Any()), msg)
			}
			return res, err
		}
	}
}
