package characters

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/characters/charactersdb"
	"github.com/PuraFome/meuRPG/backend/internal/platform/conv"
	"github.com/PuraFome/meuRPG/backend/internal/platform/db"
	"github.com/PuraFome/meuRPG/backend/internal/platform/logging"
	"github.com/PuraFome/meuRPG/backend/internal/platform/uid"
	"github.com/PuraFome/meuRPG/backend/internal/rules"
)

// The spell on a Spell Scroll (SRD 5.1) is stored on the entry of the sheet's magic items (SheetMagicItem.spell_key), in the
// sheet's JSON. SetScrollSpell sets it, a save keeps it (keepMagicItems), and every read names it for the readers who may.

// scrollSpellGonePT is what the character's player reads for a scroll whose spell the master switched off or archived after it
// was set (RN-23), in place of the spell's key and name.
const scrollSpellGonePT = rules.ScrollSpellGonePT

// scrollSpellRequest is a SetScrollSpell request whose shape is checked.
type scrollSpellRequest struct {
	id, entryID, spellKey string
	revision              int32
}

// SetScrollSpell implements charactersv1connect.CharacterServiceHandler.
func (s *Service) SetScrollSpell(
	ctx context.Context,
	req *connect.Request[charactersv1.SetScrollSpellRequest],
) (*connect.Response[charactersv1.SetScrollSpellResponse], error) {
	m, err := authz.RequireCampaignMember(ctx, req.Msg.GetCampaignId())
	if err != nil {
		return nil, err
	}
	id, ok := uid.Canonical(req.Msg.GetCharacterId())
	if !ok {
		return nil, errCharacterNotFound()
	}
	if req.Msg.GetEntryId() == "" {
		return nil, invalidArgument(fieldErr("entry_id", "is required"))
	}
	in := scrollSpellRequest{id: id, entryID: req.Msg.GetEntryId(), spellKey: req.Msg.GetSpellKey(), revision: req.Msg.GetRevision()}
	var row charactersdb.Character
	var content *rules.Content
	var changed bool
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		row, content, changed, err = s.scrollSpellIn(ctx, tx, m, in)
		return err
	})
	if err != nil {
		return nil, s.dbError(ctx, "set the spell of a scroll", err)
	}
	if changed {
		logging.Event(ctx, s.logger, "character.scroll_spell_set", slog.String("character_id", id), slog.String("entry_id", in.entryID),
			slog.String("spell_key", in.spellKey), slog.Bool("by_master", isMaster(m)), slog.String("by_user_id", m.UserID))
	}
	c, err := s.character(ctx, content, row, m)
	if err != nil {
		return nil, s.dbError(ctx, "read a character with a scroll spell", err)
	}
	return connect.NewResponse(&charactersv1.SetScrollSpellResponse{Character: c}), nil
}

// scrollSpellIn is the transaction of SetScrollSpell: the character locked, who may write, the revision, the entry, the spell
// and the write, in the order the caller is answered. It returns the table's content it checked the spell against.
func (s *Service) scrollSpellIn(ctx context.Context, tx pgx.Tx, m authz.Membership, in scrollSpellRequest) (charactersdb.Character, *rules.Content, bool, error) {
	q := s.queries.WithTx(tx)
	current, err := visibleForUpdate(ctx, q, m, in.id)
	if err != nil {
		return current, nil, false, err
	}
	// The owner writes only while the sheet is open to editing (RN-01), as a save does; the master always does.
	if !mayEditSheet(m, current) {
		return current, nil, false, errBlocked(blockedReason(characterState(current.Status, current.SheetLockedAt)), current.ID)
	}
	if in.revision != 0 && in.revision != current.Revision {
		return current, nil, false, errStaleRevision()
	}
	// The content is read in the transaction, so the spell is checked against the table as it stands now.
	content, err := s.contentFor(ctx, tx, m.CampaignID)
	if err != nil {
		return current, nil, false, wrap("read rules content", err)
	}
	stored, err := loadSheet(current.ID, current.Sheet)
	if err != nil {
		return current, nil, false, err
	}
	full := stored.GetFull()
	if full == nil {
		return current, content, false, invalidArgument(fieldErr("character_id", "is a character with a basic sheet, which has no items to hold a scroll"))
	}
	next := proto.CloneOf(full)
	i := slices.IndexFunc(next.GetMagicItems(), func(it *charactersv1.SheetMagicItem) bool { return it.GetId() == in.entryID })
	if i < 0 {
		return current, content, false, invalidArgument(fieldErr("entry_id", "is not an entry of the sheet"))
	}
	entry := next.GetMagicItems()[i]
	scroll, ok := content.SpellScrollNumbers(entry.GetKey())
	if !ok {
		return current, content, false, invalidArgument(fieldErr("entry_id", "is not a Spell Scroll"))
	}
	if err := checkScrollSpell(content, scroll, in.spellKey, isMaster(m)); err != nil {
		return current, content, false, err
	}
	if entry.GetSpellKey() == in.spellKey {
		return current, content, false, nil // already as asked: nothing changes
	}
	entry.SpellKey = in.spellKey
	row, err := s.writeScrollSpell(ctx, q, m, current, next)
	return row, content, err == nil, err
}

// checkScrollSpell refuses a spell that cannot be on the scroll: one that does not exist, one that is not of the scroll's level
// (a cantrip is 0), and one the table has switched off or archived. An empty key takes the spell off and is never refused.
// RN-23: to anyone but the master a spell that is switched off or archived is refused as a key that is no spell, and before
// the level is looked at, so the refusal tells it apart from none; the master is told it is not on.
func checkScrollSpell(content *rules.Content, scroll rules.SpellScrollNumbers, spellKey string, master bool) error {
	if spellKey == "" {
		return nil
	}
	det, ok := content.SpellDetails(spellKey)
	hidden := ok && content.Hidden(spellKey)
	switch {
	case !ok, hidden && !master:
		return invalidArgument(fieldErr("spell_key", "is not a spell"))
	case det.Spell.Level != scroll.Level:
		return invalidArgument(fieldErr("spell_key", "is not a spell of the scroll's level"))
	case hidden:
		return invalidArgument(fieldErr("spell_key", "is not on at the table"))
	}
	return nil
}

// writeScrollSpell stores the sheet, which moves the character's revision.
func (s *Service) writeScrollSpell(ctx context.Context, q *charactersdb.Queries, m authz.Membership, current charactersdb.Character, next *charactersv1.FullSheet) (charactersdb.Character, error) {
	doc, err := storeJSON.Marshal(&charactersv1.CharacterSheet{Content: &charactersv1.CharacterSheet_Full{Full: next}})
	if err != nil {
		return current, wrap("encode a sheet", err)
	}
	row, err := q.UpdateCharacterSheet(ctx, charactersdb.UpdateCharacterSheetParams{
		CampaignID: m.CampaignID, ID: current.ID, Revision: current.Revision, Name: current.Name, Sheet: doc, Now: s.now(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return current, errStaleRevision()
	}
	if err != nil {
		return current, wrap("update sheet", err)
	}
	return row, nil
}

// fillScrollSpells fills, on every read, the spell of each Spell Scroll entry and what reading it comes to for the character
// (derived is the character's): the spell's name and the consumable's scroll fields. A spell the master switched off or
// archived after it was set is never named to the character's player (RN-23): the key and the name are empty and
// spell_gone_pt says why; the master reads the key and the name with spell_off. A stored spell whose level is no longer the
// scroll's reads as no spell to the player and, to the master, as the key and the name with spell_level_changed. The scroll
// fields of the consumable are filled only for a spell that is on and of the scroll's level.
func fillScrollSpells(content *rules.Content, f *charactersv1.FullSheet, derived rules.Derived, master bool) {
	for _, it := range f.GetMagicItems() {
		key := it.GetSpellKey()
		if key == "" {
			continue
		}
		sp := content.ScrollSpellOf(it.GetKey(), key)
		if !sp.IsScroll {
			it.SpellKey = "" // a spell stored on what is no scroll says nothing
			continue
		}
		it.Consumable = sheetConsumableOf(content, it.GetKey())
		switch {
		case !master && sp.Moved:
			it.SpellKey = "" // the scroll reads as having no spell
		case !master && sp.Off:
			it.SpellKey, it.SpellGonePt = "", scrollSpellGonePT
		case master && (sp.Off || sp.Moved):
			it.SpellNamePt, it.SpellOff, it.SpellLevelChanged = content.NamePT(key), sp.Off, sp.Moved
		default:
			it.SpellNamePt = content.NamePT(key)
			fillScrollRead(it.Consumable, content, derived, key)
		}
	}
}

// fillScrollRead fills the scroll fields of a consumable with what reading the spell comes to for the character.
func fillScrollRead(c *charactersv1.SheetConsumable, content *rules.Content, derived rules.Derived, key string) {
	read, _ := content.ReadScrollSpell(derived, key)
	c.ScrollSpellLevel, c.ReadsScrollSpell, c.ScrollUnreadablePt = conv.Int32(read.Level), read.Reads, read.UnreadablePT
	c.ScrollCheckNeeded, c.ScrollCheckDc = read.CheckNeeded, conv.Int32(read.CheckDC)
	c.ScrollCheckAbility, c.ScrollCastingEconomy = abilityToProto[read.Ability], economyToProto[read.Economy]
}
