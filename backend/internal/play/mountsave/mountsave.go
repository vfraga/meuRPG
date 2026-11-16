// Package mountsave holds what the saving throw of a rider whose mount is pushed or knocked prone (SRD 5.1, Mounted Combat) keeps
// in plain words: the cause a reaction window and its event store, and the cause the API says.
package mountsave

import (
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// The causes a window keeps in its trigger.
const (
	Pushed = "pushed"
	Prone  = "prone"
)

// Key is the word a window keeps for its cause.
func Key(cause playv1.MountSaveCause) string {
	if cause == playv1.MountSaveCause_MOUNT_SAVE_CAUSE_PRONE {
		return Prone
	}
	return Pushed
}

// CauseOf is the cause a stored word says.
func CauseOf(key string) playv1.MountSaveCause {
	switch key {
	case Prone:
		return playv1.MountSaveCause_MOUNT_SAVE_CAUSE_PRONE
	case Pushed:
		return playv1.MountSaveCause_MOUNT_SAVE_CAUSE_PUSHED
	}
	return playv1.MountSaveCause_MOUNT_SAVE_CAUSE_UNSPECIFIED // the fall of a dismount: no save was asked
}
