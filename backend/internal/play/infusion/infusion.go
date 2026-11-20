// Package infusion holds what the play module knows about an Artificer's infusions that needs no store, no transport and no
// Service: the session event kinds, the payload of their events and the characters a set of changes names. play writes the
// events and tells the combat, and calls here.
package infusion

import "github.com/PuraFome/meuRPG/backend/internal/play/link"

// The session events of an Artificer's infusions (the characters module's character_infusions). Each payload holds ids and
// keys, never a name or free text. The event... names are the ones the scan of the migrations' test reads.
const (
	eventInfused = "infusion_infused"
	eventMoved   = "infusion_moved"
	eventEnded   = "infusion_ended"
)

// The kinds, as play and its tests name them.
const (
	// KindInfused: an item was infused.
	KindInfused = eventInfused
	// KindMoved: an infusion was put on another item.
	KindMoved = eventMoved
	// KindEnded: an infusion ended.
	KindEnded = eventEnded
)

// Event is the payload of the three infusion events: the record, the artificer, the holder and the infusion's option, with the
// item it is on. Ended is why a record ended (KindEnded only).
type Event struct {
	RecordID    string `json:"record_id"`
	ArtificerID string `json:"artificer_character_id"`
	HolderID    string `json:"holder_character_id"`
	Infusion    string `json:"infusion_key"`
	ItemKind    string `json:"item_kind"`
	ItemKey     string `json:"item_key"`
	Ended       string `json:"ended_reason,omitempty"`
}

// kinds are the session event kinds of the changes a write reports.
var kinds = map[link.InfusionChangeKind]string{
	link.InfusionInfused: KindInfused,
	link.InfusionMoved:   KindMoved,
	link.InfusionEnded:   KindEnded,
}

// KindOf is the session event kind of a change, and false for a change of a kind it does not know.
func KindOf(c link.InfusionChange) (string, bool) {
	k, ok := kinds[c.Kind]
	return k, ok
}

// EventOf is the payload of a change's session event.
func EventOf(c link.InfusionChange) Event {
	return Event{
		RecordID: c.RecordID, ArtificerID: c.ArtificerID, HolderID: c.HolderID, Infusion: c.InfusionKey,
		ItemKind: c.ItemKind, ItemKey: c.ItemKey, Ended: c.EndedReason,
	}
}

// Named are the characters the changes name, the artificer then the holder of each change, in the order of the call.
func Named(changes []link.InfusionChange) []string {
	ids := make([]string, 0, 2*len(changes))
	for _, c := range changes {
		ids = append(ids, c.ArtificerID, c.HolderID)
	}
	return ids
}
