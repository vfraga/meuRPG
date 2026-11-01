package play

import (
	"context"
	"errors"
	"math"

	"connectrpc.com/connect"

	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The spell slots a table's feature action moves (MR-025): an action that turns the uses it spends into a slot
// (gives_slot) and one that spends a slot to heal by its level (spends_slot). Both go through the vitals like every
// spend, write a resource line in the log with the slot's level, and the master's undo takes them back.

// What a table action's slot did (resourceEvent.Kind).
const (
	resSlotGive  = "slot_give"
	resSlotSpend = "slot_spend"
)

// giveSlot adds the free slot a gives_slot action makes, after the uses it spent: it lasts until the next long rest,
// like a slot Flexible Casting creates.
func (f *takeActionFlow) giveSlot(ctx context.Context, level, spent int) error {
	v, err := f.s.vitals.GiveSpellSlot(ctx, f.c.tx, f.m.CampaignID, f.who.CharacterID, level)
	if err != nil {
		return resourceError(err)
	}
	f.out.vitals = v
	f.made.Res = &resourceEvent{Kind: resSlotGive, SlotLevel: clamp32(level, 0, rules.MaxSlotLevel), Spent: clamp32(spent, 0, math.MaxInt32)}
	return nil
}

// spendSlot expends the free slot of the level the player picked (the `spend` of the request, 1 to 9) and returns
// the level: the heal grows with it.
func (f *takeActionFlow) spendSlot(ctx context.Context) (int, error) {
	level := int(f.call.spend)
	if level < 1 || level > rules.MaxSlotLevel {
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("spend must be the level of the spell slot, 1 to 9"))
	}
	v, err := f.s.vitals.SpendSpellSlot(ctx, f.c.tx, f.m.CampaignID, f.who.CharacterID, level)
	if err != nil {
		return 0, resourceError(err)
	}
	f.out.vitals = v
	f.made.Res = &resourceEvent{Kind: resSlotSpend, SlotLevel: clamp32(level, 0, rules.MaxSlotLevel)}
	return level, nil
}

// slotBack gives back the slot a table action made or spent, for the master's undo.
func (u *undoer) slotBack(ctx context.Context, res *resourceEvent, characterID string) error {
	campaign, level := u.c.session.CampaignID, int(res.SlotLevel)
	undo := u.s.vitals.UndoSpendSpellSlot
	if res.Kind == resSlotGive {
		undo = u.s.vitals.UndoGiveSpellSlot
	}
	v, err := undo(ctx, u.c.tx, campaign, characterID, level)
	if err != nil {
		return err
	}
	u.keep(v)
	return nil
}
