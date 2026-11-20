package infusion

import (
	"slices"
	"testing"

	"github.com/PuraFome/meuRPG/backend/internal/play/link"
)

func TestKindOfNamesTheThreeKindsAndRefusesTheRest(t *testing.T) {
	t.Parallel()
	for in, want := range map[link.InfusionChangeKind]string{
		link.InfusionInfused: KindInfused,
		link.InfusionMoved:   KindMoved,
		link.InfusionEnded:   KindEnded,
	} {
		if got, ok := KindOf(link.InfusionChange{Kind: in}); !ok || got != want {
			t.Errorf("KindOf(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := KindOf(link.InfusionChange{Kind: "other"}); ok {
		t.Error("an unknown kind has no session event kind")
	}
}

func TestEventOfCarriesIdsAndKeysOnly(t *testing.T) {
	t.Parallel()
	got := EventOf(link.InfusionChange{
		Kind: link.InfusionEnded, RecordID: "r", ArtificerID: "a", HolderID: "h", InfusionKey: "k",
		ItemKind: "gear", ItemKey: "gear:1", EndedReason: "item_gone",
	})
	want := Event{RecordID: "r", ArtificerID: "a", HolderID: "h", Infusion: "k", ItemKind: "gear", ItemKey: "gear:1", Ended: "item_gone"}
	if got != want {
		t.Errorf("EventOf = %+v, want %+v", got, want)
	}
}

func TestNamedListsTheArtificerThenTheHolderOfEachChange(t *testing.T) {
	t.Parallel()
	got := Named([]link.InfusionChange{{ArtificerID: "a", HolderID: "h"}, {ArtificerID: "b", HolderID: "b"}})
	if want := []string{"a", "h", "b", "b"}; !slices.Equal(got, want) {
		t.Errorf("Named = %v, want %v", got, want)
	}
}
