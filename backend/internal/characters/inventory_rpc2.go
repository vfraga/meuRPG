package characters

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/dice"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/platform/names"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// applyItemUpdate is the master's correction of a line.
func applyItemUpdate(t *invTx, it *charactersv1.InventoryItem, req *charactersv1.UpdateItemRequest) error {
	tr := t.rl.traits(it)
	if req.Quantity != nil {
		q := req.GetQuantity()
		if q < 1 || q > maxItemQuantity {
			return invalidArgument(fieldErr("quantity", "must be 1 to %d", maxItemQuantity))
		}
		if !tr.Stackable && q != 1 {
			return invalidArgument(fieldErr("quantity", "an item that does not stack comes one at a time"))
		}
		it.Quantity = q
	}
	if req.ChargesUsed != nil {
		if tr.Def == nil || tr.Def.Charges == nil {
			return invalidArgument(fieldErr("charges_used", "this item has no charges"))
		}
		if c := req.GetChargesUsed(); c < 0 || int(c) > tr.Def.Charges.Max {
			return invalidArgument(fieldErr("charges_used", "must be 0 to %d", tr.Def.Charges.Max))
		}
		it.ChargesUsed = req.GetChargesUsed()
	}
	if req.Name != nil {
		if it.GetCatalogKey() != "" {
			return invalidArgument(fieldErr("name", "only a free-text item has a name of its own"))
		}
		name, err := names.Clean(req.GetName(), maxItemNameLength)
		if err != nil {
			return invalidArgument(&fieldError{field: "name", err: err})
		}
		it.Name = name
	}
	if req.ScrollSpellKey != nil {
		d := tr.Def
		level, ok := t.content.SpellLevel(req.GetScrollSpellKey())
		if d == nil || d.Use == nil || !d.Use.Scroll || !ok || level != d.Use.ScrollLevel {
			return invalidArgument(fieldErr("scroll_spell_key", "must be a spell of the level the scroll holds"))
		}
		it.ScrollSpellKey = req.GetScrollSpellKey()
	}
	if err := updateIdentity(t, it, tr, req); err != nil {
		return err
	}
	t.touch()
	return nil
}

// updateIdentity reveals an item, hides it again, or changes its look.
func updateIdentity(t *invTx, it *charactersv1.InventoryItem, tr rules.ItemTraits, req *charactersv1.UpdateItemRequest) error {
	if req.Look != nil {
		look := ""
		if req.GetLook() != "" {
			var err error
			if look, err = names.Clean(req.GetLook(), maxItemNameLength); err != nil {
				return invalidArgument(&fieldError{field: "look", err: err})
			}
		}
		if tr.Kind != rules.ItemMagic {
			return invalidArgument(fieldErr("look", "only a magic item has a look"))
		}
		it.Look = look
	}
	if req.Unidentified == nil {
		return nil
	}
	if tr.Kind != rules.ItemMagic {
		return invalidArgument(fieldErr("unidentified", "only a magic item is unidentified"))
	}
	if req.GetUnidentified() && !it.GetUnidentified() {
		// Hidden again: nothing may stack with it, and the player's marks on it are dropped.
		it.Request = charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_UNSPECIFIED
	}
	if !req.GetUnidentified() && it.GetUnidentified() {
		t.event("item_identified", map[string]any{"character_id": t.doc.row.ID, "item_id": it.GetId(), "item_key": it.GetCatalogKey(), "unidentified": true})
	}
	it.Unidentified = req.GetUnidentified()
	return nil
}

// TransferItem implements charactersv1connect.InventoryServiceHandler.
func (s *Service) TransferItem(
	ctx context.Context,
	req *connect.Request[charactersv1.TransferItemRequest],
) (*connect.Response[charactersv1.TransferItemResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	from, okFrom := parseUUID(req.Msg.GetFromCharacterId())
	to, okTo := parseUUID(req.Msg.GetToCharacterId())
	if !okFrom {
		return nil, errCharacterNotFound()
	}
	if !okTo {
		return nil, invalidArgument(fieldErr("to_character_id", "is not a character of the campaign"))
	}
	if from == to {
		return nil, invalidArgument(fieldErr("to_character_id", "must be another character"))
	}
	if q := req.Msg.GetQuantity(); q < 0 || q > maxItemQuantity {
		return nil, invalidArgument(fieldErr("quantity", "must be 0 to %d", maxItemQuantity))
	}
	key, err := idem.Clean(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, invalidArgument(fieldErr("idempotency_key", "is required"))
	}
	hash := requestHash(req.Msg)
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	var resp *charactersv1.TransferItemResponse
	var last *invTx
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		resp, last = nil, nil
		if s.creatureHost != nil {
			if err := s.creatureHost.LockSession(ctx, tx, m.CampaignID); err != nil {
				return wrap("lock the session", err)
			}
		}
		q := s.queries.WithTx(tx)
		// Both rows are locked in one order, so two hands-over the other way round do not wait for each other.
		ids := []string{from, to}
		slices.Sort(ids)
		docs := map[string]*inventoryDoc{}
		for _, id := range ids {
			row, err := q.GetCharacterForUpdate(ctx, charactersdb.GetCharacterForUpdateParams{CampaignID: m.CampaignID, ID: id})
			if errors.Is(err, pgx.ErrNoRows) {
				if id == from {
					return errCharacterNotFound()
				}
				return invalidArgument(fieldErr("to_character_id", "is not a character of the campaign"))
			}
			if err != nil {
				return wrap("lock a character", err)
			}
			if id == from && !canSee(m, row.Kind, row.Status, row.PlayerUserID) {
				return errCharacterNotFound()
			}
			d, err := inventoryDocOf(row)
			if err != nil {
				if id == to {
					return invalidArgument(fieldErr("to_character_id", "cannot take items"))
				}
				return err
			}
			docs[id] = d
		}
		src, dst := docs[from], docs[to]
		t, err := s.newInvTx(ctx, tx, q, m, content, src)
		if err != nil {
			return err
		}
		last = t
		if seen, same := seenOperation(src.inv, m.UserID, key, hash); seen {
			if !same {
				return idem.ErrReused()
			}
			resp = &charactersv1.TransferItemResponse{From: t.view(src), To: t.view(dst)}
			return nil
		}
		if err := s.transfer(t, src, dst, req.Msg); err != nil {
			return err
		}
		recordOperation(src.inv, m.UserID, key, hash)
		for _, d := range []*inventoryDoc{src, dst} {
			t.notifyOf(d)
			if err := s.saveInventory(ctx, q, content, d); err != nil {
				return err
			}
		}
		if err := t.writeEvents(); err != nil {
			return err
		}
		resp = &charactersv1.TransferItemResponse{From: t.view(src), To: t.view(dst)}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "hand over an item", err)
	}
	last.publish()
	return connect.NewResponse(resp), nil
}

// transfer moves the pieces from one inventory to the other.
func (s *Service) transfer(t *invTx, src, dst *inventoryDoc, req *charactersv1.TransferItemRequest) error {
	if !t.master && !t.owner {
		return errCharacterNotFound()
	}
	if b := t.notDead(); b != nil {
		return b.err()
	}
	if dst.row.Status == statusDead || dst.row.Status == statusPending || (dst.row.Kind != kindPlayer && !t.master) {
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_CANNOT_RECEIVE, "").err()
	}
	it, err := t.itemOf(src, req.GetItemId())
	if err != nil {
		return err
	}
	n := req.GetQuantity()
	if n == 0 {
		n = it.GetQuantity()
	}
	if n > it.GetQuantity() {
		return (&itemBlock{reason: charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOT_ENOUGH, itemID: it.GetId(), have: it.GetQuantity()}).err()
	}
	if it.GetEquipped() && t.rl.traits(it).Slot == rules.SlotBody && t.rl.inCombat && !t.master {
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_IN_COMBAT, it.GetId()).err()
	}
	moved := &charactersv1.InventoryItem{
		Id: it.GetId(), CatalogKey: it.GetCatalogKey(), BaseKey: it.GetBaseKey(), OptionKey: it.GetOptionKey(), Name: it.GetName(),
		Quantity: n, Unidentified: it.GetUnidentified(), Look: it.GetLook(), ChargesUsed: it.GetChargesUsed(), ScrollSpellKey: it.GetScrollSpellKey(),
	}
	if n < it.GetQuantity() {
		moved.Id = newItemID()
		it.Quantity -= n
	} else {
		src.inv.Items = slices.DeleteFunc(src.inv.Items, func(o *charactersv1.InventoryItem) bool { return o == it })
	}
	// The receiver's rules decide whether it stacks.
	rl := invRules{content: t.content}
	line, err := rl.stackInto(dst.inv, moved)
	if err != nil {
		var full *fullError
		if errors.As(err, &full) {
			return full.block().err()
		}
		return invalidArgument(err)
	}
	src.changed, dst.changed = true, true
	t.event("item_transferred", map[string]any{
		"character_id": src.row.ID, "to_character_id": dst.row.ID, "item_id": line.GetId(), "item_key": it.GetCatalogKey(),
		"quantity": n, "unidentified": it.GetUnidentified(),
	})
	return nil
}

// RequestItemRest implements charactersv1connect.InventoryServiceHandler.
func (s *Service) RequestItemRest(
	ctx context.Context,
	req *connect.Request[charactersv1.RequestItemRestRequest],
) (*connect.Response[charactersv1.RequestItemRestResponse], error) {
	m, err := authz.RequireCampaignMemberOrPending(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	kind := req.Msg.GetKind()
	if _, ok := charactersv1.ItemRequestKind_name[int32(kind)]; !ok {
		return nil, invalidArgument(fieldErr("kind", "is not a known kind"))
	}
	view, _, err := s.editInventory(ctx, invCall{m: m, characterID: req.Msg.GetCharacterId(), key: req.Msg.GetIdempotencyKey(), request: req.Msg}, func(t *invTx) error {
		if !t.owner {
			return errPermission("only the character's player marks an item for the short rest: the master does it directly")
		}
		if b := t.notDead(); b != nil {
			return b.err()
		}
		it, err := t.itemOf(t.doc, req.Msg.GetItemId())
		if err != nil {
			return err
		}
		return markRequest(t, it, kind)
	})
	if err != nil {
		return nil, s.dbError(ctx, "mark an item for the short rest", err)
	}
	return connect.NewResponse(&charactersv1.RequestItemRestResponse{Inventory: view}), nil
}

// markRequest marks an item for the next short rest.
func markRequest(t *invTx, it *charactersv1.InventoryItem, kind charactersv1.ItemRequestKind) error {
	if kind == charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_UNSPECIFIED {
		if it.GetRequest() != kind {
			it.Request = kind
			t.touch()
		}
		return nil
	}
	if it.GetRequest() == kind {
		return nil
	}
	if it.GetRequest() != charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_UNSPECIFIED {
		return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_REQUEST_WAITING, it.GetId()).err()
	}
	unknown := !t.knownToCaller(it)
	if unknown && kind != charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_IDENTIFY {
		// The player does not know what it is: all they can ask is to learn it.
		return errUnknownItem()
	}
	var b *itemBlock
	switch kind {
	case charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_ATTUNE:
		b = t.rl.attune(t.doc.inv, it, false)
	case charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_END_ATTUNEMENT:
		b = t.rl.endAttunement(it, false)
	default:
		b = t.rl.identify(it)
	}
	if b != nil {
		return b.err()
	}
	it.Request = kind
	t.touch()
	return nil
}

// applyRest does what a short rest does to an item: attunes, ends the attunement or
// identifies. It returns the refusal, if the rules do not allow it.
func (t *invTx) applyRest(d *inventoryDoc, it *charactersv1.InventoryItem, kind charactersv1.ItemRequestKind) *itemBlock {
	rl := t.rl
	if d != t.doc {
		rl = invRules{content: t.content, who: rules.CharacterOf(buildOf(d.full), t.content), inCombat: t.rl.inCombat}
	}
	var b *itemBlock
	switch kind {
	case charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_ATTUNE:
		if b = rl.attune(d.inv, it, false); b == nil {
			wasHidden := it.GetUnidentified()
			it.Attuned, it.Unidentified = true, false // attuning teaches how the item works
			t.event("item_attuned", map[string]any{"character_id": d.row.ID, "item_id": it.GetId(), "item_key": it.GetCatalogKey(), "attuned": true, "unidentified": wasHidden})
		}
	case charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_END_ATTUNEMENT:
		if b = rl.endAttunement(it, false); b == nil {
			it.Attuned = false
			t.event("item_attuned", map[string]any{"character_id": d.row.ID, "item_id": it.GetId(), "item_key": it.GetCatalogKey(), "attuned": false})
		}
	case charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_IDENTIFY:
		if b = rl.identify(it); b == nil {
			it.Unidentified = false
			t.event("item_identified", map[string]any{"character_id": d.row.ID, "item_id": it.GetId(), "item_key": it.GetCatalogKey(), "unidentified": true})
		}
	default:
		b = block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOTHING_TO_ANSWER, it.GetId())
	}
	if b == nil {
		it.Request = charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_UNSPECIFIED
		d.changed = true
		t.notifyOf(d)
	}
	return b
}

// ResolveItemRest implements charactersv1connect.InventoryServiceHandler.
func (s *Service) ResolveItemRest(
	ctx context.Context,
	req *connect.Request[charactersv1.ResolveItemRestRequest],
) (*connect.Response[charactersv1.ResolveItemRestResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	if _, ok := charactersv1.ItemRequestKind_name[int32(req.Msg.GetKind())]; !ok {
		return nil, invalidArgument(fieldErr("kind", "is not a known kind"))
	}
	view, _, err := s.editInventory(ctx, invCall{m: m, characterID: req.Msg.GetCharacterId(), key: req.Msg.GetIdempotencyKey(), request: req.Msg}, func(t *invTx) error {
		it, err := t.itemOf(t.doc, req.Msg.GetItemId())
		if err != nil {
			return err
		}
		kind := cmp.Or(req.Msg.GetKind(), it.GetRequest())
		if kind == charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_UNSPECIFIED {
			return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOTHING_TO_ANSWER, it.GetId()).err()
		}
		if !req.Msg.GetApprove() {
			if it.GetRequest() == charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_UNSPECIFIED {
				return block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_NOTHING_TO_ANSWER, it.GetId()).err()
			}
			it.Request = charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_UNSPECIFIED
			t.touch()
			return nil
		}
		if b := t.applyRest(t.doc, it, kind); b != nil {
			return b.err()
		}
		return nil
	})
	if err != nil {
		return nil, s.dbError(ctx, "answer an item request", err)
	}
	return connect.NewResponse(&charactersv1.ResolveItemRestResponse{Inventory: view}), nil
}

// ConfirmItemRests implements charactersv1connect.InventoryServiceHandler.
func (s *Service) ConfirmItemRests(
	ctx context.Context,
	req *connect.Request[charactersv1.ConfirmItemRestsRequest],
) (*connect.Response[charactersv1.ConfirmItemRestsResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := idem.Clean(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, invalidArgument(fieldErr("idempotency_key", "is required"))
	}
	choices, err := restChoices(req.Msg.GetChoices())
	if err != nil {
		return nil, err
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	var done []*charactersv1.ItemRestDone
	var notify *invTx
	hash := requestHash(req.Msg)
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var inner error
		done, notify, inner = s.itemShortRest(ctx, tx, m, content, choices, key, hash)
		return inner
	})
	if err != nil {
		return nil, s.dbError(ctx, "confirm the items' short rest", err)
	}
	notify.publish()
	return connect.NewResponse(&charactersv1.ConfirmItemRestsResponse{Done: done}), nil
}

// ItemShortRest is the items' part of the master's short rest: every attunement, end of
// attunement and identification the players marked on the characters (all the campaign's
// when none is given) is done, inside tx, under the rules of ResolveItemRest. A request the
// rules refuse (a fourth item, a restriction not met) stays marked and comes back with the
// reason. The short rest of the party calls it in its own transaction, so the items never
// change without the rest. The caller is the campaign's master (the caller checked) and
// publish is called after the commit.
func (s *Service) ItemShortRest(ctx context.Context, tx pgx.Tx, m authz.Membership, choices []*charactersv1.ItemRestChoice) (done []*charactersv1.ItemRestDone, publish func(), err error) {
	content, err := s.contentFor(ctx, tx, m.CampaignID)
	if err != nil {
		return nil, nil, wrap("read rules content", err)
	}
	cs, err := restChoices(choices)
	if err != nil {
		return nil, nil, err
	}
	res, t, err := s.itemShortRest(ctx, tx, m, content, cs, "", "")
	if err != nil {
		return nil, nil, err
	}
	return res, t.publish, nil
}

// itemShortRest does the work for ConfirmItemRests and ItemShortRest. A key makes it
// idempotent: it is recorded on the first character only, as the call's mark.
func (s *Service) itemShortRest(ctx context.Context, tx pgx.Tx, m authz.Membership, content *rules.Content, choices map[restChoice]bool, key, hash string) ([]*charactersv1.ItemRestDone, *invTx, error) {
	if s.creatureHost != nil {
		if err := s.creatureHost.LockSession(ctx, tx, m.CampaignID); err != nil {
			return nil, nil, wrap("lock the session", err)
		}
	}
	q := s.queries.WithTx(tx)
	rows, err := q.ListCampaignSheets(ctx, m.CampaignID)
	if err != nil {
		return nil, nil, wrap("list the campaign's sheets", err)
	}
	var out []*charactersv1.ItemRestDone
	var shared *invTx
	for _, r := range rows {
		if len(choices) > 0 && !slices.ContainsFunc(slices.Collect(maps.Keys(choices)), func(c restChoice) bool { return c.character == r.ID }) {
			continue
		}
		row, err := q.GetCharacterForUpdate(ctx, charactersdb.GetCharacterForUpdateParams{CampaignID: m.CampaignID, ID: r.ID})
		if err != nil {
			return nil, nil, wrap("lock a character", err)
		}
		d, err := inventoryDocOf(row)
		if err != nil {
			continue // a basic sheet carries no items
		}
		t, err := s.newInvTx(ctx, tx, q, m, content, d)
		if err != nil {
			return nil, nil, err
		}
		if shared == nil {
			shared = t
		}
		if t.rl.inCombat {
			return nil, nil, block(charactersv1.ItemBlockedReason_ITEM_BLOCKED_REASON_IN_COMBAT, "").err()
		}
		if key != "" {
			if seen, same := seenOperation(d.inv, m.UserID, key, hash); seen {
				if !same {
					return nil, nil, idem.ErrReused()
				}
				continue
			}
		}
		pending := 0
		for _, it := range d.inv.GetItems() {
			kind := it.GetRequest()
			if kind == charactersv1.ItemRequestKind_ITEM_REQUEST_KIND_UNSPECIFIED || (len(choices) > 0 && !choices[restChoice{d.row.ID, it.GetId()}]) {
				continue
			}
			pending++
			done := &charactersv1.ItemRestDone{CharacterId: d.row.ID, ItemId: it.GetId(), Kind: kind, NamePt: content.ItemName(ruleItem(it))}
			if b := t.applyRest(d, it, kind); b != nil {
				done.Blocked = b.reason
			}
			out = append(out, done)
		}
		// The names above are the master's: an item attuned has been revealed by now.
		for _, e := range out {
			if e.CharacterId == d.row.ID {
				if it, _ := findItem(d.inv, e.ItemId); it != nil {
					e.NamePt = content.ItemName(ruleItem(it))
				}
			}
		}
		if pending > 0 && key != "" {
			recordOperation(d.inv, m.UserID, key, hash)
		}
		if err := s.saveInventory(ctx, q, content, d); err != nil {
			return nil, nil, err
		}
		if shared != t {
			shared.events = append(shared.events, t.events...)
			for id, o := range t.notify {
				shared.notify[id] = o
			}
		}
	}
	if shared == nil {
		shared = &invTx{s: s, m: m, notify: map[string]string{}}
		return out, shared, nil
	}
	if err := shared.writeEvents(); err != nil {
		return nil, nil, err
	}
	return out, shared, nil
}

// RegainCharges implements charactersv1connect.InventoryServiceHandler.
func (s *Service) RegainCharges(
	ctx context.Context,
	req *connect.Request[charactersv1.RegainChargesRequest],
) (*connect.Response[charactersv1.RegainChargesResponse], error) {
	m, err := authz.RequireCampaignRole(ctx, req.Msg.GetCampaignId(), authz.RoleMaster)
	if err != nil {
		return nil, err
	}
	key, err := idem.Clean(req.Msg.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, invalidArgument(fieldErr("idempotency_key", "is required"))
	}
	var only string
	if req.Msg.GetCharacterId() != "" {
		id, ok := parseUUID(req.Msg.GetCharacterId())
		if !ok {
			return nil, errCharacterNotFound()
		}
		only = id
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	hash := requestHash(req.Msg)
	var regained []*charactersv1.ChargesRegained
	var last *invTx
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		regained, last = nil, nil
		inner := &invTx{}
		var err error
		regained, inner, err = s.regainCharges(ctx, tx, m, content, only, key, hash)
		last = inner
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "regain charges", err)
	}
	last.publish()
	return connect.NewResponse(&charactersv1.RegainChargesResponse{Regained: regained}), nil
}

// RegainItemCharges is dawn for the items inside tx: the long rest of the party calls it
// in its own transaction, so the charges never come back without the rest. publish is
// called after the commit.
func (s *Service) RegainItemCharges(ctx context.Context, tx pgx.Tx, m authz.Membership, characterIDs []string) (regained []*charactersv1.ChargesRegained, publish func(), err error) {
	content, err := s.contentFor(ctx, tx, m.CampaignID)
	if err != nil {
		return nil, nil, wrap("read rules content", err)
	}
	var all []*charactersv1.ChargesRegained
	var notify []*invTx
	for _, id := range characterIDs {
		r, t, err := s.regainCharges(ctx, tx, m, content, id, "", "")
		if err != nil {
			return nil, nil, err
		}
		all, notify = append(all, r...), append(notify, t)
	}
	return all, func() {
		for _, t := range notify {
			t.publish()
		}
	}, nil
}

func (s *Service) regainCharges(ctx context.Context, tx pgx.Tx, m authz.Membership, content *rules.Content, only, key, hash string) ([]*charactersv1.ChargesRegained, *invTx, error) {
	if s.creatureHost != nil {
		if err := s.creatureHost.LockSession(ctx, tx, m.CampaignID); err != nil {
			return nil, nil, wrap("lock the session", err)
		}
	}
	q := s.queries.WithTx(tx)
	rows, err := q.ListCampaignSheets(ctx, m.CampaignID)
	if err != nil {
		return nil, nil, wrap("list the campaign's sheets", err)
	}
	var out []*charactersv1.ChargesRegained
	shared := &invTx{s: s, m: m, ctx: ctx, tx: tx, notify: map[string]string{}}
	for _, r := range rows {
		if only != "" && r.ID != only {
			continue
		}
		row, err := q.GetCharacterForUpdate(ctx, charactersdb.GetCharacterForUpdateParams{CampaignID: m.CampaignID, ID: r.ID})
		if err != nil {
			return nil, nil, wrap("lock a character", err)
		}
		d, err := inventoryDocOf(row)
		if err != nil {
			continue
		}
		if key != "" {
			if seen, same := seenOperation(d.inv, m.UserID, key, hash); seen {
				if !same {
					return nil, nil, idem.ErrReused()
				}
				continue
			}
		}
		for _, it := range d.inv.GetItems() {
			def := content.Traits(ruleItem(it)).Def
			if def == nil || def.Charges == nil || def.Charges.RegainDice == "" || it.GetChargesUsed() == 0 {
				continue
			}
			expr, err := dice.Parse(def.Charges.RegainDice)
			if err != nil {
				return nil, nil, wrap("read the dice an item regains", err)
			}
			res, err := dice.Roll(s.roller, expr)
			if err != nil {
				return nil, nil, wrap("roll the charges an item regains", err)
			}
			got := min(int32(max(res.Total, 0)), it.GetChargesUsed())
			it.ChargesUsed -= got
			d.changed = true
			out = append(out, &charactersv1.ChargesRegained{
				CharacterId: d.row.ID, ItemId: it.GetId(), NamePt: content.ItemName(ruleItem(it)), Regained: got, ChargesUsed: it.GetChargesUsed(),
			})
			shared.events = append(shared.events, itemEvent{kind: "item_charges", payload: map[string]any{
				"character_id": d.row.ID, "item_id": it.GetId(), "item_key": it.GetCatalogKey(), "unidentified": it.GetUnidentified(), "amount": got,
			}})
		}
		if d.changed {
			if key != "" {
				recordOperation(d.inv, m.UserID, key, hash)
			}
			shared.notifyOf(d)
		}
		if err := s.saveInventory(ctx, q, content, d); err != nil {
			return nil, nil, err
		}
	}
	if err := shared.writeEvents(); err != nil {
		return nil, nil, err
	}
	return out, shared, nil
}

// itemLog builds the master's item log.
func (s *Service) itemLog(ctx context.Context, m authz.Membership, req *charactersv1.ListItemLogRequest) (*connect.Response[charactersv1.ListItemLogResponse], error) {
	limit := int(req.GetLimit())
	if limit < 0 || limit > 100 {
		return nil, invalidArgument(fieldErr("limit", "must be 0 to 100"))
	}
	if limit == 0 {
		limit = 50
	}
	only := ""
	if req.GetCharacterId() != "" {
		id, ok := parseUUID(req.GetCharacterId())
		if !ok {
			return nil, errCharacterNotFound()
		}
		only = id
	}
	if s.creatureHost == nil {
		return connect.NewResponse(&charactersv1.ListItemLogResponse{}), nil
	}
	content, err := s.contentFor(ctx, nil, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "read rules content", err)
	}
	events, err := s.creatureHost.ItemEvents(ctx, m.CampaignID, 100)
	if err != nil {
		return nil, s.dbError(ctx, "read the item events", err)
	}
	rows, err := s.queries.ListCampaignSheets(ctx, m.CampaignID)
	if err != nil {
		return nil, s.dbError(ctx, "list the campaign's sheets", err)
	}
	sheets := map[string]*charactersv1.Inventory{}
	charNames := map[string]string{}
	for _, r := range rows {
		charNames[r.ID] = r.Name
		if sh, err := loadSheet(r.ID, r.Sheet); err == nil && sh.GetFull() != nil {
			sheets[r.ID] = sh.GetFull().GetInventory()
		}
	}
	out := &charactersv1.ListItemLogResponse{}
	for _, e := range events {
		var p struct {
			CharacterID  string `json:"character_id"`
			ToCharacter  string `json:"to_character_id"`
			TargetID     string `json:"target_id"`
			ItemID       string `json:"item_id"`
			ItemKey      string `json:"item_key"`
			Quantity     int32  `json:"quantity"`
			Unidentified bool   `json:"unidentified"`
			Amount       int32  `json:"amount"`
			ByMaster     bool   `json:"by_master"`
		}
		if json.Unmarshal(e.Payload, &p) != nil || (only != "" && p.CharacterID != only && p.ToCharacter != only) {
			continue
		}
		ent := &charactersv1.ItemLogEntry{
			Kind: logKinds[e.Kind], At: timestamppb.New(e.At), CharacterId: p.CharacterID, CharacterName: charNames[p.CharacterID],
			ToCharacterId: p.ToCharacter, ToCharacterName: charNames[p.ToCharacter], Quantity: p.Quantity, Unidentified: p.Unidentified, Amount: p.Amount, ByMaster: p.ByMaster,
		}
		ent.ItemNamePt = itemLogName(content, sheets, p.CharacterID, p.ToCharacter, p.ItemID, p.ItemKey, p.Unidentified, isMaster(m))
		out.Entries = append(out.Entries, ent)
		if len(out.Entries) == limit {
			break
		}
	}
	return connect.NewResponse(out), nil
}

var logKinds = map[string]charactersv1.ItemLogKind{
	"item_given":       charactersv1.ItemLogKind_ITEM_LOG_KIND_GIVEN,
	"item_transferred": charactersv1.ItemLogKind_ITEM_LOG_KIND_TRANSFERRED,
	"item_used":        charactersv1.ItemLogKind_ITEM_LOG_KIND_USED,
	"item_attuned":     charactersv1.ItemLogKind_ITEM_LOG_KIND_ATTUNED,
	"item_charges":     charactersv1.ItemLogKind_ITEM_LOG_KIND_CHARGES,
	"item_identified":  charactersv1.ItemLogKind_ITEM_LOG_KIND_IDENTIFIED,
}

// itemLogName names the item of a log line. The master reads the true name. A player reads it
// as the character sees it: the look while the item is unidentified (and, when the item is gone
// and was unidentified, no name at all), the name once it is known.
func itemLogName(content *rules.Content, sheets map[string]*charactersv1.Inventory, from, to, itemID, key string, wasHidden, master bool) string {
	for _, c := range []string{to, from} {
		if it, _ := findItem(sheets[c], itemID); it != nil {
			if !master && it.GetUnidentified() {
				return content.ItemName(rules.Item{Key: it.GetCatalogKey(), Unidentified: true, Look: it.GetLook()})
			}
			ri := ruleItem(it)
			ri.Unidentified = false // the master reads what it is
			if !master {
				ri.Unidentified = it.GetUnidentified()
			}
			return content.ItemName(ri)
		}
	}
	if key != "" && (master || !wasHidden) {
		return content.ItemName(rules.Item{Key: key})
	}
	return ""
}

// restChoice names one marked item of one character.
type restChoice struct{ character, item string }

// restChoices reads the master's choices; none means everything marked.
func restChoices(in []*charactersv1.ItemRestChoice) (map[restChoice]bool, error) {
	if len(in) > rules.MaxInventoryItems {
		return nil, invalidArgument(fieldErr("choices", "has too many entries"))
	}
	out := map[restChoice]bool{}
	for i, c := range in {
		ch, ok1 := parseUUID(c.GetCharacterId())
		it, ok2 := parseUUID(c.GetItemId())
		if !ok1 || !ok2 {
			return nil, invalidArgument(fieldErr("choices", "entry %d does not name a character and an item", i))
		}
		out[restChoice{ch, it}] = true
	}
	return out, nil
}
