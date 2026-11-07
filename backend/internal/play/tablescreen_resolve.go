package play

import (
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The table screen decides what the TV shows in three single places, each one function:
//   - resolveStage: which screen (the mirror order, the pin and its fallbacks);
//   - mapOnTV: whether a map's content may go to the TV;
//   - tvShowsConversation: whether the Conversa goes to the TV.

// tvShowsConversation is false: the Conversa (RN-36) never goes to the TV, and the narration to
// the whole table is not shown on it either.
const tvShowsConversation = false

// tvFacts is what resolveStage needs to know of the session, already read.
type tvFacts struct {
	// ImageToTable: the session shows an image whose audience is the whole table.
	ImageToTable bool
	// CombatActive: a combat is in progress.
	CombatActive bool
	// SceneOpen: a scene is open.
	SceneOpen bool
	// SceneForAll: the open scene is open to the whole table. Meaningful only with SceneOpen.
	SceneForAll bool
	// HasMap: the session has a current map.
	HasMap bool
	// FogOn and GroupVision are the current map's. Meaningful only with HasMap.
	FogOn       bool
	GroupVision bool
}

// tvChoice is the master's stored choice.
type tvChoice struct {
	Mirror bool
	Pin    playv1.TableScreenKind
}

// tvPick is the screen resolveStage chose and why it is not the one asked for.
type tvPick struct {
	Kind   playv1.TableScreenKind
	Notice playv1.TableScreenNotice
}

// mapOnTV says whether a map's content may go to the TV: not when the fog of war is on and "Visão do
// grupo" is off, because the TV would show what no player sees together (RN-10).
func mapOnTV(fogOn, groupVision bool) bool { return !fogOn || groupVision }

// tvUnavailable says why the TV cannot show the item now, or UNSPECIFIED when it can. The idle
// screen is always available.
func tvUnavailable(kind playv1.TableScreenKind, f tvFacts) playv1.TableScreenUnavailable {
	switch kind {
	case playv1.TableScreenKind_TABLE_SCREEN_KIND_IMAGE:
		if !f.ImageToTable {
			return playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_NO_IMAGE
		}
	case playv1.TableScreenKind_TABLE_SCREEN_KIND_COMBAT:
		if !f.CombatActive {
			return playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_NO_COMBAT
		}
	case playv1.TableScreenKind_TABLE_SCREEN_KIND_SCENE:
		if !f.SceneOpen {
			return playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_NO_SCENE
		}
		if !f.SceneForAll {
			return playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_SCENE_NOT_FOR_ALL
		}
	case playv1.TableScreenKind_TABLE_SCREEN_KIND_MAP:
		if !f.HasMap {
			return playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_NO_MAP
		}
		if !mapOnTV(f.FogOn, f.GroupVision) {
			return playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_NEEDS_GROUP_VISION
		}
	}
	return playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_UNSPECIFIED
}

// tvPinnable lists the items the master can pick, in the order the panel shows them.
var tvPinnable = []playv1.TableScreenKind{
	playv1.TableScreenKind_TABLE_SCREEN_KIND_IDLE,
	playv1.TableScreenKind_TABLE_SCREEN_KIND_MAP,
	playv1.TableScreenKind_TABLE_SCREEN_KIND_IMAGE,
	playv1.TableScreenKind_TABLE_SCREEN_KIND_SCENE,
	playv1.TableScreenKind_TABLE_SCREEN_KIND_COMBAT,
}

// tvOptions are the items of the master's panel and whether each can be picked now.
func tvOptions(f tvFacts) []*playv1.TableScreenOption {
	out := make([]*playv1.TableScreenOption, 0, len(tvPinnable))
	for _, k := range tvPinnable {
		why := tvUnavailable(k, f)
		out = append(out, &playv1.TableScreenOption{
			Kind:        k,
			Available:   why == playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_UNSPECIFIED,
			Unavailable: why,
		})
	}
	return out
}

// resolveStage decides which screen the TV shows.
//
// Mirror, first that exists: the image shown to the whole table, the combat, the scene open to the
// whole table, the map the TV may show, the idle screen. The master's own tabs do not count.
// A scene open to only some players, or a map hidden by the fog, is skipped with a notice.
//
// Pin: the pinned item when it can be shown; otherwise the idle screen, with the notice that says
// why. The choice is never written by this function.
func resolveStage(c tvChoice, f tvFacts) tvPick {
	if c.Mirror {
		return mirrorStage(f)
	}
	return pinnedStage(c.Pin, f)
}

func mirrorStage(f tvFacts) tvPick {
	switch {
	case f.ImageToTable:
		return tvPick{Kind: playv1.TableScreenKind_TABLE_SCREEN_KIND_IMAGE}
	case f.CombatActive:
		return tvPick{Kind: playv1.TableScreenKind_TABLE_SCREEN_KIND_COMBAT}
	case f.SceneOpen && f.SceneForAll:
		return tvPick{Kind: playv1.TableScreenKind_TABLE_SCREEN_KIND_SCENE}
	}
	notice := playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_UNSPECIFIED
	if f.SceneOpen { // not for all, or it would have been chosen above
		notice = playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_SCENE_NOT_FOR_ALL
	}
	if f.HasMap {
		if mapOnTV(f.FogOn, f.GroupVision) {
			return tvPick{Kind: playv1.TableScreenKind_TABLE_SCREEN_KIND_MAP, Notice: notice}
		}
		if notice == playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_UNSPECIFIED {
			notice = playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_MAP_NEEDS_GROUP_VISION
		}
	}
	return tvPick{Kind: playv1.TableScreenKind_TABLE_SCREEN_KIND_IDLE, Notice: notice}
}

func pinnedStage(pin playv1.TableScreenKind, f tvFacts) tvPick {
	idle := tvPick{Kind: playv1.TableScreenKind_TABLE_SCREEN_KIND_IDLE}
	if pin == playv1.TableScreenKind_TABLE_SCREEN_KIND_IDLE || !validTvPin(pin) {
		return idle
	}
	switch tvUnavailable(pin, f) {
	case playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_UNSPECIFIED:
		return tvPick{Kind: pin}
	case playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_SCENE_NOT_FOR_ALL:
		idle.Notice = playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_SCENE_NOT_FOR_ALL
	case playv1.TableScreenUnavailable_TABLE_SCREEN_UNAVAILABLE_NEEDS_GROUP_VISION:
		idle.Notice = playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_MAP_NEEDS_GROUP_VISION
	default:
		idle.Notice = playv1.TableScreenNotice_TABLE_SCREEN_NOTICE_PIN_ENDED
	}
	return idle
}

// The values of game_sessions.tv_pin: the enum's own numbers, which the column check allows (1 to 5).
const (
	tvPinIdle int16 = iota + 1
	tvPinMap
	tvPinImage
	tvPinScene
	tvPinCombat
)

// tvPinColumn is the column value of each item the master can pin.
var tvPinColumn = map[playv1.TableScreenKind]int16{
	playv1.TableScreenKind_TABLE_SCREEN_KIND_IDLE:   tvPinIdle,
	playv1.TableScreenKind_TABLE_SCREEN_KIND_MAP:    tvPinMap,
	playv1.TableScreenKind_TABLE_SCREEN_KIND_IMAGE:  tvPinImage,
	playv1.TableScreenKind_TABLE_SCREEN_KIND_SCENE:  tvPinScene,
	playv1.TableScreenKind_TABLE_SCREEN_KIND_COMBAT: tvPinCombat,
}

// validTvPin says whether kind is one the master can pin.
func validTvPin(kind playv1.TableScreenKind) bool {
	for _, k := range tvPinnable {
		if k == kind {
			return true
		}
	}
	return false
}
