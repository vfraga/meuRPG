package play

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// namingRoster names two keys; "feature:retired" is one the master retired, so it has no name.
type namingRoster struct{ CombatRoster }

func (namingRoster) SourceNames(context.Context, pgx.Tx, string) (func(string) string, error) {
	return func(k string) string {
		if k == "feat:dado-sortudo" {
			return "Dado Sortudo da Mesa"
		}
		return ""
	}, nil
}

func TestNameSourcesFillsEveryKeyAResponseCarries(t *testing.T) {
	t.Parallel()
	s := &Service{roster: namingRoster{}, logger: slog.New(slog.DiscardHandler)}
	roll := &playv1.DiceRoll{
		TreatedAsSource: "feat:dado-sortudo",
		Rerolled:        []*playv1.Reroll{{SourceKey: "feat:dado-sortudo"}, {SourceKey: "feature:retired"}, {}},
	}
	offer := &playv1.HeldRollOffer{D20: roll, Question: &playv1.HeldRollOffer_Reroll{Reroll: &playv1.RerollQuestion{SourceKey: "feat:dado-sortudo"}}}
	s.nameSources(t.Context(), "campaign", offer)
	if roll.GetTreatedAsSourceName() != "Dado Sortudo da Mesa" || roll.GetRerolled()[0].GetSourceName() != "Dado Sortudo da Mesa" || offer.GetReroll().GetSourceName() != "Dado Sortudo da Mesa" {
		t.Errorf("names = %q %q %q, want the feature's name on the roll, its reroll and the question", roll.GetTreatedAsSourceName(), roll.GetRerolled()[0].GetSourceName(), offer.GetReroll().GetSourceName())
	}
	if roll.GetRerolled()[1].GetSourceName() != "" || roll.GetRerolled()[2].GetSourceName() != "" {
		t.Errorf("a retired entry or no key got a name: %v", roll.GetRerolled())
	}
}

func TestNameSourcesNamesNothingWithoutAKeyOrACampaign(t *testing.T) {
	t.Parallel()
	s := &Service{roster: namingRoster{}, logger: slog.New(slog.DiscardHandler)}
	roll := &playv1.DiceRoll{Rerolled: []*playv1.Reroll{{SourceKey: "feat:dado-sortudo"}}}
	s.nameSources(t.Context(), "", roll)
	if roll.GetRerolled()[0].GetSourceName() != "" {
		t.Errorf("a response with no campaign got a name")
	}
}

func TestRerollQuestionCarriesTheD20Sides(t *testing.T) {
	t.Parallel()
	q := heldOffer{Hold: heldRoll{RerollSource: "feat:x"}}.rerollOffer()
	if q.GetSides() != 20 {
		t.Errorf("sides = %d, want 20: the prompt draws the d20 from it", q.GetSides())
	}
	out := heldRollOfferOf(heldOffer{Asks: askReroll, Hold: heldRoll{Faces: []int{5}}}, offerRoller{})
	if out.GetD20().GetDiceSides() != 20 || out.GetReroll().GetSides() != 20 {
		t.Errorf("offer sides = %d and %d, want 20 on the d20 and on the question", out.GetD20().GetDiceSides(), out.GetReroll().GetSides())
	}
}

// TestEveryNamedSourcePairIsInTheTable fails when a message of the protos has a source key with a sibling name field that
// namedSourceFields does not list, or the table lists a pair that no longer exists.
func TestEveryNamedSourcePairIsInTheTable(t *testing.T) {
	t.Parallel()
	seen := map[protoreflect.FullName]bool{}
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		var walk func(ms protoreflect.MessageDescriptors)
		walk = func(ms protoreflect.MessageDescriptors) {
			for i := range ms.Len() {
				md := ms.Get(i)
				walk(md.Messages())
				fields := md.Fields()
				for j := range fields.Len() {
					k := fields.Get(j)
					n := string(k.Name())
					if k.Kind() != protoreflect.StringKind || k.IsList() || (n != "source_key" && !strings.HasSuffix(n, "treated_as_source")) {
						continue
					}
					want := n + "_name"
					if n == "source_key" {
						want = "source_name"
					}
					if fields.ByName(protoreflect.Name(want)) == nil {
						continue
					}
					seen[md.FullName()] = true
					if !slices.Contains(namedSourceFields[md.FullName()], sourceNamePair{k.Name(), protoreflect.Name(want)}) {
						t.Errorf("%s has %s with %s, which namedSourceFields does not list: no name would be filled", md.FullName(), n, want)
					}
				}
			}
		}
		walk(fd.Messages())
		return true
	})
	for name := range namedSourceFields {
		if !seen[name] {
			t.Errorf("namedSourceFields lists %s, which has no such pair in the protos", name)
		}
	}
}

// TestNameSourcesWalksListsAndMaps names the keys inside a list (the held offers) and inside a map of messages.
func TestNameSourcesWalksListsAndMaps(t *testing.T) {
	t.Parallel()
	s := &Service{roster: namingRoster{}, logger: slog.New(slog.DiscardHandler)}
	key := "feat:dado-sortudo"
	list := &playv1.ListHeldRollsResponse{Offers: []*playv1.HeldRollOffer{
		{D20: &playv1.DiceRoll{TreatedAsSource: key}},
		{Question: &playv1.HeldRollOffer_Reroll{Reroll: &playv1.RerollQuestion{SourceKey: key}}},
	}}
	s.nameSources(t.Context(), "campaign", list)
	if list.GetOffers()[0].GetD20().GetTreatedAsSourceName() == "" || list.GetOffers()[1].GetReroll().GetSourceName() == "" {
		t.Errorf("a list of offers was not named: %v", list)
	}

	// A message with a map of Reroll, which no proto of the play service has.
	rerollFile := (&playv1.Reroll{}).ProtoReflect().Descriptor().ParentFile()
	str, msg, rep, opt := descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, descriptorpb.FieldDescriptorProto_LABEL_REPEATED, descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	fdp := &descriptorpb.FileDescriptorProto{
		Name: proto.String("test/maproll.proto"), Package: proto.String("test"), Syntax: proto.String("proto3"),
		Dependency: []string{rerollFile.Path()},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name:  proto.String("Holder"),
			Field: []*descriptorpb.FieldDescriptorProto{{Name: proto.String("by_id"), Number: proto.Int32(1), Label: &rep, Type: &msg, TypeName: proto.String(".test.Holder.ByIdEntry"), JsonName: proto.String("byId")}},
			NestedType: []*descriptorpb.DescriptorProto{{
				Name:    proto.String("ByIdEntry"),
				Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("key"), Number: proto.Int32(1), Label: &opt, Type: &str},
					{Name: proto.String("value"), Number: proto.Int32(2), Label: &opt, Type: &msg, TypeName: proto.String(".meurpg.play.v1.Reroll")},
				},
			}},
		}},
	}
	file, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatal(err)
	}
	md := file.Messages().ByName("Holder")
	holder := dynamicpb.NewMessage(md)
	byID := holder.Mutable(md.Fields().ByName("by_id")).Map()
	byID.Set(protoreflect.ValueOfString("a").MapKey(), protoreflect.ValueOfMessage((&playv1.Reroll{SourceKey: key}).ProtoReflect()))
	s.nameSources(t.Context(), "campaign", holder)
	got := byID.Get(protoreflect.ValueOfString("a").MapKey()).Message().Interface().(*playv1.Reroll)
	if got.GetSourceName() != "Dado Sortudo da Mesa" {
		t.Errorf("the reroll in a map was not named: %v", got)
	}
}

// TestSourceNameInterceptorNamesTheAnswer runs a call through the interceptor: the answer comes back named, and a call
// with an error or no campaign is left alone.
func TestSourceNameInterceptorNamesTheAnswer(t *testing.T) {
	t.Parallel()
	s := &Service{roster: namingRoster{}, logger: slog.New(slog.DiscardHandler)}
	answer := func(_ context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		return connect.NewResponse(&playv1.HeldRollOffer{D20: &playv1.DiceRoll{Rerolled: []*playv1.Reroll{{SourceKey: "feat:dado-sortudo"}}}}), nil
	}
	call := func(campaign string) *playv1.HeldRollOffer {
		res, err := sourceNameInterceptor(s)(answer)(t.Context(), connect.NewRequest(&playv1.ListHeldRollsRequest{CampaignId: campaign}))
		if err != nil {
			t.Fatal(err)
		}
		return res.Any().(*playv1.HeldRollOffer)
	}
	if got := call("campaign").GetD20().GetRerolled()[0].GetSourceName(); got != "Dado Sortudo da Mesa" {
		t.Errorf("name = %q, want the feature's", got)
	}
	if got := call("").GetD20().GetRerolled()[0].GetSourceName(); got != "" {
		t.Errorf("name = %q with no campaign, want none", got)
	}
}

// BenchmarkNameSources measures what the interceptor adds to an answer: one with no key to name, one that cannot hold a
// key at all, and a list of 50 offers that all name a feature.
func BenchmarkNameSources(b *testing.B) {
	s := &Service{roster: namingRoster{}, logger: slog.New(slog.DiscardHandler)}
	var offers []*playv1.HeldRollOffer
	for range 50 {
		offers = append(offers, &playv1.HeldRollOffer{D20: &playv1.DiceRoll{Rerolled: []*playv1.Reroll{{SourceKey: "feat:dado-sortudo"}}}})
	}
	for name, msg := range map[string]proto.Message{
		"cannot hold a key": &playv1.EndGameSessionRequest{},
		"no key":            &playv1.ListHeldRollsResponse{Offers: []*playv1.HeldRollOffer{{D20: &playv1.DiceRoll{Faces: []int32{5}}}}},
		"50 offers named":   &playv1.ListHeldRollsResponse{Offers: offers},
	} {
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				s.nameSources(b.Context(), "campaign", msg)
			}
		})
	}
}
