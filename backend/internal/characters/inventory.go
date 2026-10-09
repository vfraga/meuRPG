package characters

import (
	"crypto/sha256"
	"fmt"
	"math"
	"slices"
	"strings"
	"uuid"

	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The inventory (inventory.proto): the lines a character carries, kept in the full
// sheet but owned by the server. This file holds how it is stored and kept; the
// rules it follows are in inventory_rules.go and the calls in inventory_rpc.go.

// maxInventoryOperations is how many idempotency keys a character keeps.
const maxInventoryOperations = 100

// attunementMax is how many magic items a character is attuned to at most (SRD 5.1
// "Attunement").
const attunementMax = 3

// itemsOf turns the stored inventory into the lines package rules reads.
func itemsOf(inv *charactersv1.Inventory) []rules.Item {
	var out []rules.Item
	for _, it := range inv.GetItems() {
		out = append(out, rules.Item{
			ID: it.GetId(), Key: it.GetCatalogKey(), Base: it.GetBaseKey(), Option: it.GetOptionKey(), Name: it.GetName(),
			Quantity: int(it.GetQuantity()), Equipped: it.GetEquipped(), Attuned: it.GetAttuned(),
			Unidentified: it.GetUnidentified(), Look: it.GetLook(),
		})
	}
	return out
}

// alignmentOf is the good or evil side of an alignment, for the attunement
// restrictions that name one.
func alignmentOf(a charactersv1.Alignment) string {
	switch a {
	case charactersv1.Alignment_ALIGNMENT_LAWFUL_GOOD, charactersv1.Alignment_ALIGNMENT_NEUTRAL_GOOD, charactersv1.Alignment_ALIGNMENT_CHAOTIC_GOOD:
		return rules.AlignmentGood
	case charactersv1.Alignment_ALIGNMENT_LAWFUL_EVIL, charactersv1.Alignment_ALIGNMENT_NEUTRAL_EVIL, charactersv1.Alignment_ALIGNMENT_CHAOTIC_EVIL:
		return rules.AlignmentEvil
	}
	return ""
}

// derivedID is a UUID made from the words, the same every time: the id of a line
// the server read from an old sheet's equipment text, so it is the same on every
// read until the sheet is saved with the inventory.
func derivedID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	var b [16]byte
	copy(b[:], sum[:16])
	b[6] = b[6]&0x0f | 0x50 // version 5 layout
	b[8] = b[8]&0x3f | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// foldEquipment moves the free-text equipment lines of a sheet into its inventory,
// word for word, and empties the lines. A line whose name an item of the inventory
// has already is not added twice (an old editor sending its lines back). It returns
// whether it changed anything.
func foldEquipment(characterID string, f *charactersv1.FullSheet) bool {
	if len(f.GetEquipment()) == 0 {
		return false
	}
	if f.Inventory == nil {
		f.Inventory = &charactersv1.Inventory{}
	}
	for i, line := range f.GetEquipment() {
		name := line.GetName()
		if name == "" || slices.ContainsFunc(f.Inventory.Items, func(it *charactersv1.InventoryItem) bool {
			return it.GetCatalogKey() == "" && it.GetName() == name
		}) {
			continue
		}
		f.Inventory.Items = append(f.Inventory.Items, &charactersv1.InventoryItem{
			Id: derivedID(characterID, "equipment", fmt.Sprint(i), name), Name: name, Quantity: max(line.GetQuantity(), 1),
		})
	}
	f.Equipment = nil
	return true
}

// keepInventory makes the stored inventory the only one on a sheet about to be
// written: what the client sent in `inventory` is dropped, and the free-text
// equipment lines it sent join the stored inventory. stored is nil for a new sheet.
func keepInventory(characterID string, stored, next *charactersv1.CharacterSheet) {
	f := next.GetFull()
	if f == nil {
		return
	}
	f.Inventory = nil
	if stored.GetFull() != nil {
		f.Inventory = proto.CloneOf(stored.GetFull().GetInventory())
	}
	foldEquipment(characterID, f)
	if f.GetInventory() != nil && len(f.Inventory.Items) == 0 && len(f.Inventory.Operations) == 0 {
		f.Inventory = nil
	}
}

// withoutInventory is a copy of a sheet as the app may read it: the inventory is
// left out, so an item the master has not identified cannot leak through it; the app
// reads it with InventoryService.GetInventory.
func hideInventory(s *charactersv1.CharacterSheet) {
	if f := s.GetFull(); f != nil {
		f.Inventory = nil
	}
}

// inventoryOf returns the inventory of a full sheet, creating it empty.
func inventoryOf(f *charactersv1.FullSheet) *charactersv1.Inventory {
	if f.Inventory == nil {
		f.Inventory = &charactersv1.Inventory{}
	}
	return f.Inventory
}

// findItem returns the line with the id, and its index.
func findItem(inv *charactersv1.Inventory, id string) (*charactersv1.InventoryItem, int) {
	i := slices.IndexFunc(inv.GetItems(), func(it *charactersv1.InventoryItem) bool { return it.GetId() == id })
	if i < 0 {
		return nil, -1
	}
	return inv.Items[i], i
}

// newItemID makes the id of a new line.
func newItemID() string { return uuid.New().String() }

// operationRecord is how an idempotency key is kept: the caller, the key and the
// hash of the request.
func operationRecord(userID, key string, hash string) string { return userID + ":" + key + "|" + hash }

// seenOperation says whether the character already did the call with this key, and
// whether it was this same request. A key of another caller is another key.
func seenOperation(inv *charactersv1.Inventory, userID, key, hash string) (seen, same bool) {
	prefix := userID + ":" + key + "|"
	for _, op := range inv.GetOperations() {
		if rest, ok := strings.CutPrefix(op, prefix); ok {
			return true, rest == hash
		}
	}
	return false, false
}

// recordOperation keeps a key, forgetting the oldest beyond the limit.
func recordOperation(inv *charactersv1.Inventory, userID, key, hash string) {
	inv.Operations = append(inv.Operations, operationRecord(userID, key, hash))
	if over := len(inv.Operations) - maxInventoryOperations; over > 0 {
		inv.Operations = slices.Delete(inv.Operations, 0, over)
	}
}

// clamp32 is n as an int32, held inside its range.
func clamp32(n int) int32 {
	return int32(min(max(n, math.MinInt32), math.MaxInt32))
}
