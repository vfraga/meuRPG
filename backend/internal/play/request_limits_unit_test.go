package play

import (
	"strconv"
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The id lists of a request are checked against their limit before any id is parsed or looked up: the ids
// here do not exist (and are not even UUIDs), so a request that got past the size check would be not_found.

func unknownIDs(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = "unknown-" + strconv.Itoa(i)
	}
	return ids
}

func wantInvalidArgument(t *testing.T, err error) {
	t.Helper()
	if got := connect.CodeOf(err); err == nil || got != connect.CodeInvalidArgument {
		t.Fatalf("error = %v, want invalid_argument", err)
	}
}

func TestPreserveLifeOverTheLimitIsRefusedBeforeAnyLookup(t *testing.T) {
	t.Parallel()
	for _, n := range []int{maxCombatants + 1, 100_000} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			t.Parallel()
			heals := make([]*playv1.PreserveLifeHeal, n)
			for i, id := range unknownIDs(n) {
				heals[i] = &playv1.PreserveLifeHeal{CombatantId: id, HitPoints: 1}
			}
			const id = "00000000-0000-4000-8000-000000000001" // nothing with this id exists
			_, err := readTakeActionCall(&playv1.TakeActionRequest{
				IdempotencyKey: id, EncounterId: id, CombatantId: id, ActionKey: preserveLifeAction, PreserveLife: heals,
			})
			wantInvalidArgument(t, err)
		})
	}
}

func TestInitiativeOrderOverTheLimitIsRefused(t *testing.T) {
	t.Parallel()
	_, err := orderIDs(unknownIDs(maxCombatants + 1))
	wantInvalidArgument(t, err)
}

func TestGroupCheckAskOverTheLimitIsRefused(t *testing.T) {
	t.Parallel()
	s := &Service{roster: namedRoster{}}
	_, err := s.parseGroupAsk(&playv1.RequestGroupCheckRequest{SkillKey: "skill:stealth", CharacterIds: unknownIDs(maxCombatants + 1)})
	wantInvalidArgument(t, err)
}

func TestResolveHideOverTheLimitIsRefused(t *testing.T) {
	t.Parallel()
	_, err := parseResolveHidePlan(&playv1.ResolveHideRequest{AttemptId: "00000000-0000-4000-8000-000000000001", SeesClearlyIds: unknownIDs(maxCombatants + 1)})
	wantInvalidArgument(t, err)
}

// namedRoster knows every check key by a name.
type namedRoster struct{ noRoster }

func (namedRoster) SceneCheckName(string) string { return "check" }
