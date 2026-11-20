package link

// InfusionChangeKind says what happened to the record of an infusion.
type InfusionChangeKind string

// The changes of a record a write reports (characters.ReviewHost.InfusionChanged).
const (
	// InfusionInfused: a record began on an item.
	InfusionInfused InfusionChangeKind = "infused"
	// InfusionMoved: a live record went to another item.
	InfusionMoved InfusionChangeKind = "moved"
	// InfusionEnded: a record ended.
	InfusionEnded InfusionChangeKind = "ended"
)

// InfusionChange is one change of one record. It holds ids and keys only: the record, the artificer, the holder, the
// infusion's option key and the item (a kind word and its key), and why the record ended (InfusionEnded only).
type InfusionChange struct {
	Kind        InfusionChangeKind
	RecordID    string
	ArtificerID string
	HolderID    string
	InfusionKey string
	ItemKind    string
	ItemKey     string
	EndedReason string
}
