package play

import (
	"maps"
	"slices"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// heldRequestTypes are the requests each kind of held roll keeps and runs again (heldRegistry's rerun funcs unmarshal them). A
// reaction window's replay is answered under the answer's own key (underKey, and rerunAttack), which a request without an
// idempotency_key string field would silently ignore: the path would find the event that held the request, take the run for a
// retry and write nothing. So every request kept has the field.
var heldRequestTypes = map[heldKind][]proto.Message{
	heldWeaponAttack:      {&playv1.RollAttackRequest{}},
	heldSpellAttack:       {&playv1.CastSpellRequest{}},
	heldEffectSave:        {&playv1.RollEffectSaveRequest{}},
	heldConcentrationSave: {&playv1.ResolveConcentrationSaveRequest{}},
	heldSceneCheck:        {&playv1.RollSceneCheckRequest{}},
	heldGroupCheck:        {&playv1.RollGroupCheckRequest{}, &playv1.RollForPlayerRequest{}},
	heldTrapSearch:        {&playv1.SearchForTrapsRequest{}},
	heldHintTry:           {&playv1.TryPuzzleHintRequest{}},
	heldDeathSave:         {&playv1.RollDeathSaveRequest{}},
	heldHide:              {&playv1.HideRequest{}},
	heldCounterspell:      {&playv1.AnswerReactionRequest{}},
	heldContest:           {&playv1.StartContestRequest{}, &playv1.RespondContestRequest{}},
	heldInitiative:        {&playv1.SubmitInitiativeRequest{}},
}

// TestEveryHeldKindKeepsARequestWithAnIdempotencyKey: heldRequestTypes has a row for every kind of heldRegistry (and no other), and
// every request of the row has an idempotency_key field of type string, which is what underKey sets.
func TestEveryHeldKindKeepsARequestWithAnIdempotencyKey(t *testing.T) {
	t.Parallel()
	registry := slices.Sorted(maps.Keys(heldRegistry))
	table := slices.Sorted(maps.Keys(heldRequestTypes))
	if !slices.Equal(registry, table) {
		t.Fatalf("heldRequestTypes kinds = %v, want the kinds of heldRegistry: %v (a new kind needs its request here)", table, registry)
	}
	for _, kind := range registry {
		reqs := heldRequestTypes[kind]
		if len(reqs) == 0 {
			t.Errorf("%s: no request type", kind)
		}
		for _, req := range reqs {
			name := string(req.ProtoReflect().Descriptor().FullName())
			f := req.ProtoReflect().Descriptor().Fields().ByName("idempotency_key")
			if f == nil || f.Kind() != protoreflect.StringKind || f.IsList() {
				t.Errorf("%s: %s has no string idempotency_key field: underKey would leave a window's replay under the key of the request that held it", kind, name)
			}
		}
	}
}
