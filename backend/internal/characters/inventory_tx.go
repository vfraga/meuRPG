package characters

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The transaction every call that changes an inventory runs in: it locks the
// character's row, reads the sheet, lets the call change the inventory, and writes the
// sheet back with a new revision, the history events in the same transaction. The
// closure is run again on a 40001, so it reads everything it needs from the
// transaction and keeps nothing between attempts.

// inventoryDoc is a character's sheet read for the inventory.
type inventoryDoc struct {
	row   charactersdb.Character
	sheet *charactersv1.CharacterSheet
	full  *charactersv1.FullSheet
	inv   *charactersv1.Inventory
	// before is the stored sheet, to carry the hit points when the maximum moves.
	before []byte
	// changed says the call changed the sheet and it must be written.
	changed bool
}

// itemEvent is a line for the session's history (a session event), with IDs and
// numbers only.
type itemEvent struct {
	kind    string
	payload map[string]any
}

// invTx is what a call sees inside its transaction.
type invTx struct {
	s       *Service
	ctx     context.Context
	tx      pgx.Tx
	q       *charactersdb.Queries
	m       authz.Membership
	content *rules.Content
	doc     *inventoryDoc
	rl      invRules
	master  bool
	// owner says the caller plays this character.
	owner bool
	// events and notify are what the call asks to be done with the commit: history
	// lines now, and the characters whose streams to tell after it.
	events []itemEvent
	notify map[string]string // character id -> owner user id
	// extra is what a call hands back (a potion's outcome, a wand's d20).
	outcome   *charactersv1.UseOutcome
	d20       int32
	destroyed bool
	rested    []*charactersv1.ItemRestDone
}

// lockInventory reads and locks a character's row and its sheet. The character must
// be visible to the caller (otherwise not_found) and have a full sheet.
func (s *Service) lockInventory(ctx context.Context, q *charactersdb.Queries, m authz.Membership, id string) (*inventoryDoc, error) {
	row, err := visibleForUpdate(ctx, q, m, id)
	if err != nil {
		return nil, err
	}
	return inventoryDocOf(row)
}

func inventoryDocOf(row charactersdb.Character) (*inventoryDoc, error) {
	sheet, err := loadSheet(row.ID, row.Sheet)
	if err != nil {
		return nil, err
	}
	full := sheet.GetFull()
	if full == nil {
		return nil, invalidArgument(fieldErr("character_id", "has a basic sheet, which carries no items"))
	}
	return &inventoryDoc{row: row, sheet: sheet, full: full, inv: inventoryOf(full), before: row.Sheet}, nil
}

// saveInventory writes the sheet when the call changed it.
func (s *Service) saveInventory(ctx context.Context, q *charactersdb.Queries, content *rules.Content, d *inventoryDoc) error {
	if !d.changed {
		return nil
	}
	doc, err := storeJSON.Marshal(d.sheet)
	if err != nil {
		return wrap("encode a sheet", err)
	}
	if _, err := q.UpdateCharacterSheet(ctx, charactersdb.UpdateCharacterSheetParams{
		CampaignID: deref(d.row.CampaignID), ID: d.row.ID, Revision: d.row.Revision, Name: d.row.Name, Sheet: doc, Now: s.now(),
	}); err != nil {
		return wrap("update the inventory", err)
	}
	return s.carryHitPoints(ctx, q, content, d.row.ID, d.before, doc)
}

// newInvTx builds the context of a call on a locked document.
func (s *Service) newInvTx(ctx context.Context, tx pgx.Tx, q *charactersdb.Queries, m authz.Membership, content *rules.Content, d *inventoryDoc) (*invTx, error) {
	inCombat := false
	if s.creatureHost != nil {
		var err error
		if inCombat, err = s.creatureHost.CampaignInCombat(ctx, tx, m.CampaignID); err != nil {
			return nil, wrap("find out whether a combat is going on", err)
		}
	}
	t := &invTx{s: s, ctx: ctx, tx: tx, q: q, m: m, content: content, doc: d, master: isMaster(m), notify: map[string]string{}}
	t.owner = !t.master && d.row.PlayerUserID != nil && *d.row.PlayerUserID == m.UserID
	t.rl = invRules{content: content, who: rules.CharacterOf(buildOf(d.full), content), inCombat: inCombat}
	return t, nil
}

// requireMaster is the answer for a call only the master makes.
func (t *invTx) requireMaster() error {
	if !t.master {
		return errPermission("only the master does this")
	}
	return nil
}

// notDead refuses a player's change to a dead character's inventory.
func (t *invTx) notDead() *itemBlock {
	if t.doc.row.Status == statusDead && !t.master {
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_DEAD, "")
	}
	return nil
}

// mayAdd says the caller may add items: the master, and the owner of a living character
// (who adds equipment and free text; a magic item is the master's gift).
func (t *invTx) mayAdd() bool { return t.master || t.owner }

// mayRemove says the caller may take the line away: the master always, the owner only a line
// they know and that is not magic (free text or SRD equipment).
func (t *invTx) mayRemove(it *charactersv1.InventoryItem) bool {
	return t.master || (t.owner && (it.GetCatalogKey() == "" || strings.HasPrefix(it.GetCatalogKey(), "equipment:")))
}

// touch marks the character as changed and to be told about.
func (t *invTx) touch() {
	t.doc.changed = true
	t.notifyOf(t.doc)
}

func (t *invTx) notifyOf(d *inventoryDoc) { t.notify[d.row.ID] = deref(d.row.PlayerUserID) }

func (t *invTx) event(kind string, payload map[string]any) {
	payload["by_master"] = t.master
	t.events = append(t.events, itemEvent{kind: kind, payload: payload})
}

// view is the inventory of a document as the caller may see it.
func (t *invTx) view(d *inventoryDoc) *charactersv1.InventoryView {
	visible := t.master || (d.row.PlayerUserID != nil && *d.row.PlayerUserID == t.m.UserID)
	if !visible {
		return nil
	}
	rl := t.rl
	if d != t.doc {
		rl = invRules{content: t.content, who: rules.CharacterOf(buildOf(d.full), t.content), inCombat: t.rl.inCombat}
	}
	canEdit := t.master || (visible && d.row.Status != statusDead)
	return viewOf(t.content, rl, d.full, d.row.ID, t.master, canEdit)
}

// invCall is what a call tells editInventory.
type invCall struct {
	m           authz.Membership
	characterID string
	key         string
	request     proto.Message
	// keyRequired makes the idempotency key mandatory (a call that is not a plain
	// setting of a state).
	keyRequired bool
}

// editInventory runs fn on one character's inventory in a transaction and answers the
// inventory as the caller may see it. A key seen before with the same request answers
// without running fn.
func (s *Service) editInventory(ctx context.Context, c invCall, fn func(t *invTx) error) (*charactersv1.InventoryView, *invTx, error) {
	id, ok := parseUUID(c.characterID)
	if !ok {
		return nil, nil, errCharacterNotFound()
	}
	key, err := idem.Clean(c.key)
	if err != nil {
		return nil, nil, err
	}
	if key == "" && c.keyRequired {
		return nil, nil, invalidArgument(fieldErr("idempotency_key", "is required"))
	}
	hash := requestHash(c.request)
	content, err := s.contentFor(ctx, nil, c.m.CampaignID)
	if err != nil {
		return nil, nil, s.dbError(ctx, "read rules content", err)
	}
	var (
		view *charactersv1.InventoryView
		last *invTx
	)
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		view, last = nil, nil
		if s.creatureHost != nil { // one lock order with the combat's writes: the session first
			if err := s.creatureHost.LockSession(ctx, tx, c.m.CampaignID); err != nil {
				return wrap("lock the session", err)
			}
		}
		q := s.queries.WithTx(tx)
		d, err := s.lockInventory(ctx, q, c.m, id)
		if err != nil {
			return err
		}
		t, err := s.newInvTx(ctx, tx, q, c.m, content, d)
		if err != nil {
			return err
		}
		last = t
		if key != "" {
			if seen, same := seenOperation(d.inv, c.m.UserID, key, hash); seen {
				if !same {
					return idem.ErrReused()
				}
				view = t.view(d)
				return nil
			}
		}
		if err := fn(t); err != nil {
			return err
		}
		if key != "" && d.changed {
			recordOperation(d.inv, c.m.UserID, key, hash)
		}
		if err := s.saveInventory(ctx, q, content, d); err != nil {
			return err
		}
		if err := t.writeEvents(); err != nil {
			return err
		}
		view = t.view(d)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	last.publish()
	return view, last, nil
}

// requestHash is the short hash a key is kept with: the first 16 hex digits of the hash
// of the whole request but its key.
func requestHash(req proto.Message) string {
	h := *idem.Hash(req)
	return h[:min(16, len(h))]
}

// writeEvents appends the call's lines to the session's history, in its transaction.
func (t *invTx) writeEvents() error {
	if t.s.creatureHost == nil {
		return nil
	}
	for _, e := range t.events {
		body, err := json.Marshal(e.payload)
		if err != nil {
			return fmt.Errorf("encode an item event: %w", err)
		}
		if _, err := t.s.creatureHost.AppendEvent(t.ctx, t.tx, t.m.CampaignID, e.kind, t.m.UserID, body, t.s.now()); err != nil {
			return wrap("write an item event", err)
		}
	}
	return nil
}

// publish tells the streams, after the commit.
func (t *invTx) publish() {
	if t.s.creatureHost == nil {
		return
	}
	for id, owner := range t.notify {
		t.s.creatureHost.PublishInventoryChanged(t.m.CampaignID, id, owner)
	}
}

// errUnknownItem is the answer for a call a player makes on an item they cannot
// know: the same whatever the item is.
func errUnknownItem() error {
	return invalidArgument(fieldErr("item_id", "is not an item you can do this with"))
}

// itemOf finds a line or answers not_found-like invalid_argument.
func (t *invTx) itemOf(d *inventoryDoc, id string) (*charactersv1.InventoryItem, error) {
	it, _ := findItem(d.inv, id)
	if it == nil {
		return nil, invalidArgument(fieldErr("item_id", "is not an item of this inventory"))
	}
	return it, nil
}

// knownToCaller says the caller knows what the item is: the master always, a player
// once it is identified.
func (t *invTx) knownToCaller(it *charactersv1.InventoryItem) bool {
	return t.master || !it.GetUnidentified()
}

// blockedErr turns a rules refusal into the error, or nil.
func blockedErr(b *itemBlock) error {
	if b == nil {
		return nil
	}
	return b.err()
}
