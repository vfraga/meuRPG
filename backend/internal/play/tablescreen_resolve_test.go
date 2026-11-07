package play

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

const (
	kIdle   = playv1.TableScreenKind_TABLE_SCREEN_KIND_IDLE
	kMap    = playv1.TableScreenKind_TABLE_SCREEN_KIND_MAP
	kImage  = playv1.TableScreenKind_TABLE_SCREEN_KIND_IMAGE
	kScene  = playv1.TableScreenKind_TABLE_SCREEN_KIND_SCENE
	kCombat = playv1.TableScreenKind_TABLE_SCREEN_KIND_COMBAT

	nNone     = playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_UNSPECIFIED
	nPinEnded = playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_PIN_ENDED
	nScene    = playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_SCENE_NOT_FOR_ALL
	nFog      = playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_MAP_NEEDS_GROUP_VISION
)

// everything is a session that has all five items, the scene open to the whole table and the map in plain sight.
func everything() tvFacts {
	return tvFacts{ImageToTable: true, CombatActive: true, SceneOpen: true, SceneForAll: true, HasMap: true}
}

func TestMapOnTV(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ fog, group, want bool }{
		{false, false, true},
		{false, true, true},
		{true, false, false},
		{true, true, true},
	} {
		if got := mapOnTV(c.fog, c.group); got != c.want {
			t.Errorf("mapOnTV(fog %v, group vision %v) = %v, want %v", c.fog, c.group, got, c.want)
		}
	}
}

func TestTVNeverShowsTheConversation(t *testing.T) {
	t.Parallel()
	if tvShowsConversation {
		t.Error("the Conversa must not go to the TV")
	}
}

func TestResolveStageMirrorOrder(t *testing.T) {
	t.Parallel()
	f := everything()
	steps := []struct {
		name  string
		drop  func(*tvFacts)
		want  playv1.TableScreenKind
		notes playv1.TableScreenNotice
	}{
		{"all five: the image wins", func(*tvFacts) {}, kImage, nNone},
		{"no image: the combat", func(f *tvFacts) { f.ImageToTable = false }, kCombat, nNone},
		{"no combat: the scene", func(f *tvFacts) { f.CombatActive = false }, kScene, nNone},
		{"no scene: the map", func(f *tvFacts) { f.SceneOpen = false }, kMap, nNone},
		{"no map: idle", func(f *tvFacts) { f.HasMap = false }, kIdle, nNone},
	}
	for _, s := range steps {
		s.drop(&f)
		got := resolveStage(tvChoice{Mirror: true}, f)
		if got.Kind != s.want || got.Notice != s.notes {
			t.Errorf("%s: stage = %v/%v, want %v/%v", s.name, got.Kind, got.Notice, s.want, s.notes)
		}
	}
}

func TestResolveStageMirrorSkipsWhatTheTableDoesNotSee(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		facts tvFacts
		want  playv1.TableScreenKind
		note  playv1.TableScreenNotice
	}{
		{"a scene for some players, a map: the map, with the notice", tvFacts{SceneOpen: true, HasMap: true}, kMap, nScene},
		{"a scene for some players, nothing else: idle, with the notice", tvFacts{SceneOpen: true}, kIdle, nScene},
		{"a scene for some players under a combat: the combat, no notice", tvFacts{SceneOpen: true, CombatActive: true}, kCombat, nNone},
		{"a fogged map, no group vision: idle, with the notice", tvFacts{HasMap: true, FogOn: true}, kIdle, nFog},
		{"a fogged map with group vision: the map", tvFacts{HasMap: true, FogOn: true, GroupVision: true}, kMap, nNone},
		{"both: the scene's notice", tvFacts{SceneOpen: true, HasMap: true, FogOn: true}, kIdle, nScene},
		{"nothing", tvFacts{}, kIdle, nNone},
	}
	for _, c := range cases {
		got := resolveStage(tvChoice{Mirror: true}, c.facts)
		if got.Kind != c.want || got.Notice != c.note {
			t.Errorf("%s: stage = %v/%v, want %v/%v", c.name, got.Kind, got.Notice, c.want, c.note)
		}
	}
}

func TestResolveStageEachPin(t *testing.T) {
	t.Parallel()
	for _, pin := range []playv1.TableScreenKind{kIdle, kMap, kImage, kScene, kCombat} {
		got := resolveStage(tvChoice{Pin: pin}, everything())
		if got.Kind != pin || got.Notice != nNone {
			t.Errorf("pin %v with everything on: stage = %v/%v, want the pin", pin, got.Kind, got.Notice)
		}
	}
}

func TestResolveStagePinFallsToIdleWithItsNotice(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		pin   playv1.TableScreenKind
		facts tvFacts
		note  playv1.TableScreenNotice
	}{
		{"the image stopped", kImage, tvFacts{CombatActive: true}, nPinEnded},
		{"the combat ended", kCombat, tvFacts{ImageToTable: true}, nPinEnded},
		{"no scene open", kScene, tvFacts{}, nPinEnded},
		{"the scene is for some players", kScene, tvFacts{SceneOpen: true}, nScene},
		{"no map", kMap, tvFacts{}, nPinEnded},
		{"the map is fogged", kMap, tvFacts{HasMap: true, FogOn: true}, nFog},
	}
	for _, c := range cases {
		got := resolveStage(tvChoice{Pin: c.pin}, c.facts)
		if got.Kind != kIdle || got.Notice != c.note {
			t.Errorf("%s: stage = %v/%v, want idle/%v", c.name, got.Kind, got.Notice, c.note)
		}
	}
	// The pin comes back when its item does: nothing was reset.
	if got := resolveStage(tvChoice{Pin: kCombat}, tvFacts{CombatActive: true}); got.Kind != kCombat {
		t.Errorf("a new combat under the old pin: stage = %v, want the combat", got.Kind)
	}
}

func TestResolveStageIgnoresAPinWhileMirroringAndAnInvalidPin(t *testing.T) {
	t.Parallel()
	if got := resolveStage(tvChoice{Mirror: true, Pin: kCombat}, tvFacts{HasMap: true}); got.Kind != kMap {
		t.Errorf("mirror with a stale combat pin: stage = %v, want the map", got.Kind)
	}
	for _, pin := range []playv1.TableScreenKind{playv1.TableScreenKind_TABLE_SCREEN_KIND_UNSPECIFIED, 99} {
		if got := resolveStage(tvChoice{Pin: pin}, everything()); got.Kind != kIdle || got.Notice != nNone {
			t.Errorf("pin %v: stage = %v/%v, want idle with no notice", pin, got.Kind, got.Notice)
		}
	}
}

func TestTVOptionsSayWhyAnItemCannotBePicked(t *testing.T) {
	t.Parallel()
	got := map[playv1.TableScreenKind]*playv1.TableScreenOption{}
	for _, o := range tvOptions(tvFacts{SceneOpen: true, HasMap: true, FogOn: true}) {
		got[o.GetKind()] = o
	}
	want := map[playv1.TableScreenKind]playv1.TableScreenUnavailable{
		kIdle:   playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_UNSPECIFIED,
		kMap:    playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_NEEDS_GROUP_VISION,
		kImage:  playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_NO_IMAGE,
		kScene:  playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_SCENE_NOT_FOR_ALL,
		kCombat: playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_NO_COMBAT,
	}
	if len(got) != len(want) {
		t.Fatalf("options = %d, want %d", len(got), len(want))
	}
	for kind, why := range want {
		o := got[kind]
		if o.GetUnavailable() != why || o.GetAvailable() != (why == playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_UNSPECIFIED) {
			t.Errorf("option %v = available %v, %v; want reason %v", kind, o.GetAvailable(), o.GetUnavailable(), why)
		}
	}
}

func TestTvPinColumnIsTheEnumNumber(t *testing.T) {
	t.Parallel()
	if len(tvPinColumn) != len(tvPinnable) {
		t.Fatalf("tvPinColumn has %d items, tvPinnable %d", len(tvPinColumn), len(tvPinnable))
	}
	for _, kind := range tvPinnable {
		column, ok := tvPinColumn[kind]
		if !ok || int32(column) != int32(kind) {
			t.Errorf("tvPinColumn[%v] = %d, want %d", kind, column, int32(kind))
		}
	}
}
