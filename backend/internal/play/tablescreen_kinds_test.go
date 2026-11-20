package play

import playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"

// The screens and the notices the table screen tests name by a short word.
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
