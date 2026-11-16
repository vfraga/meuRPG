package mountsave

import (
	"testing"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

func TestTheCauseGoesToItsWordAndBack(t *testing.T) {
	t.Parallel()
	for _, cause := range []playv1.MountSaveCause{playv1.MountSaveCause_MOUNT_SAVE_CAUSE_PUSHED, playv1.MountSaveCause_MOUNT_SAVE_CAUSE_PRONE} {
		if got := CauseOf(Key(cause)); got != cause {
			t.Errorf("CauseOf(Key(%v)) = %v, want the same cause", cause, got)
		}
	}
	if Key(playv1.MountSaveCause_MOUNT_SAVE_CAUSE_PUSHED) != Pushed || Key(playv1.MountSaveCause_MOUNT_SAVE_CAUSE_PRONE) != Prone {
		t.Errorf("the words = %q and %q, want %q and %q", Key(playv1.MountSaveCause_MOUNT_SAVE_CAUSE_PUSHED), Key(playv1.MountSaveCause_MOUNT_SAVE_CAUSE_PRONE), Pushed, Prone)
	}
	// An unspecified cause is stored as a push; a word no save wrote (a dismount's "mount_prone") says no cause.
	if Key(playv1.MountSaveCause_MOUNT_SAVE_CAUSE_UNSPECIFIED) != Pushed {
		t.Errorf("Key(unspecified) = %q, want %q", Key(playv1.MountSaveCause_MOUNT_SAVE_CAUSE_UNSPECIFIED), Pushed)
	}
	if got := CauseOf("mount_prone"); got != playv1.MountSaveCause_MOUNT_SAVE_CAUSE_UNSPECIFIED {
		t.Errorf(`CauseOf("mount_prone") = %v, want unspecified`, got)
	}
}
