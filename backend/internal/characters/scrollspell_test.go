package characters

import (
	"context"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

const (
	scrollCantrip = "item:spell-scroll-cantrip"
	scrollFirst   = "item:spell-scroll-1st"
	scrollSecond  = "item:spell-scroll-2nd"
	scrollThird   = "item:spell-scroll-3rd"

	scrollGone = "A magia deste pergaminho não está mais na mesa."
)

// scrollTable is a table with Dona's wizard (level 3) holding the scrolls, saved by the master.
func scrollTable(t *testing.T, keys ...string) (*editorTable, *charactersv1.Character) {
	t.Helper()
	et := newEditorTable(t, nil)
	pc := et.dona.createPensantus(t, et.campaign)
	req := editorSaveOf(pc)
	req.Sheet = editorClone(pc.GetSheet(), func(f *charactersv1.FullSheet) {
		for _, k := range keys {
			f.MagicItems = append(f.MagicItems, &charactersv1.SheetMagicItem{Key: k})
		}
	})
	saved, err := editorUpdate(et.master, req)
	if err != nil {
		t.Fatalf("the master puts the scrolls on the sheet: %v", err)
	}
	return et, saved
}

// setScroll calls SetScrollSpell as u, with revision 0 (no check).
func (et *editorTable) setScroll(u *user, c *charactersv1.Character, entryID, spell string) (*charactersv1.Character, error) {
	return et.setScrollAt(u, c, entryID, spell, 0)
}

func (et *editorTable) setScrollAt(u *user, c *charactersv1.Character, entryID, spell string, revision int32) (*charactersv1.Character, error) {
	res, err := u.api.SetScrollSpell(context.Background(), connect.NewRequest(&charactersv1.SetScrollSpellRequest{
		CampaignId: et.campaign, CharacterId: c.GetId(), EntryId: entryID, SpellKey: spell, Revision: revision,
	}))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetCharacter(), nil
}

func entryOf(t *testing.T, c *charactersv1.Character, key string) *charactersv1.SheetMagicItem {
	t.Helper()
	it := magicItemOf(c, key)
	if it == nil {
		t.Fatalf("the sheet has no %s", key)
	}
	return it
}

func invalidArg(field, msg string) string {
	return `invalid_argument: ` + field + ` ` + msg + ` | InvalidField{field="` + field + `"}`
}

// The master puts a spell of the scroll's level on it, the response carries the new revision and the spell's name, and an
// empty key takes it off.
func TestSetScrollSpellPutsAndTakesOffTheSpell(t *testing.T) {
	t.Parallel()
	et, pc := scrollTable(t, scrollThird, scrollCantrip)
	third, cantrip := entryOf(t, pc, scrollThird), entryOf(t, pc, scrollCantrip)
	if third.GetSpellKey() != "" || third.GetSpellNamePt() != "" {
		t.Fatalf("a new scroll reads spell %q/%q", third.GetSpellKey(), third.GetSpellNamePt())
	}
	got, err := et.setScroll(et.master, pc, third.GetId(), "spell:fireball")
	if err != nil {
		t.Fatalf("the master puts Fireball on the 3rd level scroll: %v", err)
	}
	it := entryOf(t, got, scrollThird)
	if it.GetSpellKey() != "spell:fireball" || it.GetSpellNamePt() != "Bola de Fogo" || got.GetRevision() != pc.GetRevision()+1 {
		t.Errorf("scroll reads %q/%q at revision %d, want spell:fireball/Bola de Fogo at %d", it.GetSpellKey(), it.GetSpellNamePt(), got.GetRevision(), pc.GetRevision()+1)
	}
	if other := entryOf(t, got, scrollCantrip); other.GetSpellKey() != "" {
		t.Errorf("the other scroll reads %q", other.GetSpellKey())
	}
	if got, err = et.setScroll(et.master, got, cantrip.GetId(), "spell:fire-bolt"); err != nil {
		t.Fatalf("a cantrip on the cantrip scroll: %v", err)
	}
	if read := et.read(t, et.dona, pc.GetId()); entryOf(t, read, scrollThird).GetSpellKey() != "spell:fireball" || entryOf(t, read, scrollCantrip).GetSpellKey() != "spell:fire-bolt" {
		t.Errorf("the owner reads %q and %q", entryOf(t, read, scrollThird).GetSpellKey(), entryOf(t, read, scrollCantrip).GetSpellKey())
	}
	if got, err = et.setScroll(et.master, got, third.GetId(), ""); err != nil {
		t.Fatalf("taking the spell off: %v", err)
	}
	if it := entryOf(t, got, scrollThird); it.GetSpellKey() != "" || it.GetSpellNamePt() != "" || it.GetConsumable().GetScrollSpellLevel() != 0 {
		t.Errorf("after taking it off the scroll reads %q/%q", it.GetSpellKey(), it.GetSpellNamePt())
	}
	if entryOf(t, got, scrollCantrip).GetSpellKey() != "spell:fire-bolt" {
		t.Error("taking the spell off one scroll took it off the other")
	}
}

// The owner puts a spell on a scroll while the sheet is open to editing and the master always does; another player never
// reaches the character, and the owner of a locked or dead character's sheet is told why.
func TestSetScrollSpellWhoMayAsk(t *testing.T) {
	t.Parallel()
	et, pc := scrollTable(t, scrollThird)
	id := entryOf(t, pc, scrollThird).GetId()
	_, err := et.setScroll(et.caio, pc, id, "spell:fireball")
	wantEditorRefusal(t, "another player", err, `not_found: character not found`)
	_, err = et.setScroll(et.caio, pc, "m99", "")
	wantEditorRefusal(t, "another player, an entry that is not there", err, `not_found: character not found`)
	if _, err = et.setScroll(et.visitor, pc, id, "spell:fireball"); err == nil {
		t.Error("a stranger to the campaign put a spell on the scroll")
	}
	pc, err = et.setScroll(et.dona, pc, id, "spell:fireball")
	if err != nil {
		t.Fatalf("the owner on an open sheet: %v", err)
	}
	et.h.lockSheets(et.campaign)
	locked := `failed_precondition: the sheet is locked: a game session has started, so only the master edits it | CharacterBlocked{reason=CHARACTER_BLOCKED_REASON_SHEET_LOCKED character_id="` + pc.GetId() + `"}`
	_, err = et.setScroll(et.dona, pc, id, "")
	wantEditorRefusal(t, "the owner on a locked sheet", err, locked)
	_, err = et.setScroll(et.dona, pc, "m99", "")
	wantEditorRefusal(t, "the owner on a locked sheet, an entry that is not there", err, locked)
	if pc, err = et.setScroll(et.master, pc, id, "spell:lightning-bolt"); err != nil {
		t.Fatalf("the master on a locked sheet: %v", err)
	}
	pc = et.master.markDead(t, pc)
	_, err = et.setScroll(et.dona, pc, id, "")
	wantEditorRefusal(t, "the dead character's owner", err, `failed_precondition: the character is dead, so only the master edits its sheet | CharacterBlocked{reason=CHARACTER_BLOCKED_REASON_CHARACTER_DEAD character_id="`+pc.GetId()+`"}`)
	if _, err = et.setScroll(et.master, pc, id, "spell:fireball"); err != nil {
		t.Errorf("the master on a dead character: %v", err)
	}
}

// Each check has its own refusal, in order: the revision, the entry, the scroll, the spell, its level, the table.
func TestSetScrollSpellRefusals(t *testing.T) {
	t.Parallel()
	et, pc := scrollTable(t, scrollThird, scrollCantrip, "item:bag-of-holding")
	third, cantrip, bag := entryOf(t, pc, scrollThird).GetId(), entryOf(t, pc, scrollCantrip).GetId(), entryOf(t, pc, "item:bag-of-holding").GetId()
	off := et.master.addEntry(t, et.campaign, choiceTableSpell(3, false, "class:wizard")).GetKey()
	et.master.setOff(t, et.campaign, true, "spell:lightning-bolt")
	if _, err := et.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: et.campaign, Key: off})); err != nil {
		t.Fatalf("ArchiveTableEntry: %v", err)
	}
	for _, u := range []*user{et.master, et.dona} {
		who := map[*user]string{et.master: "the master", et.dona: "the owner"}[u]
		for _, c := range []struct {
			name  string
			entry string
			spell string
			rev   int32
			want  string
		}{
			{"a stale revision", third, "spell:fireball", pc.GetRevision() + 5, editorStale},
			{"an entry that is not on the sheet", "m99", "spell:fireball", 0, invalidArg("entry_id", "is not an entry of the sheet")},
			{"an item that is no scroll", bag, "spell:fireball", 0, invalidArg("entry_id", "is not a Spell Scroll")},
			{"a key that is no spell", third, "spell:nope", 0, invalidArg("spell_key", "is not a spell")},
			{"a key of another kind", third, "item:bag-of-holding", 0, invalidArg("spell_key", "is not a spell")},
			{"a spell of another level", third, "spell:magic-missile", 0, invalidArg("spell_key", "is not a spell of the scroll's level")},
			{"a cantrip on a scroll of the 3rd level", third, "spell:fire-bolt", 0, invalidArg("spell_key", "is not a spell of the scroll's level")},
			{"a spell on a cantrip scroll", cantrip, "spell:magic-missile", 0, invalidArg("spell_key", "is not a spell of the scroll's level")},
			{"a spell the master switched off", third, "spell:lightning-bolt", 0, invalidArg("spell_key", "is not on at the table")},
			{"a spell of the table the master archived", third, off, 0, invalidArg("spell_key", "is not on at the table")},
		} {
			_, err := et.setScrollAt(u, pc, c.entry, c.spell, c.rev)
			wantEditorRefusal(t, who+": "+c.name, err, c.want)
		}
	}
	read := et.read(t, et.master, pc.GetId())
	if read.GetRevision() != pc.GetRevision() || entryOf(t, read, scrollThird).GetSpellKey() != "" {
		t.Errorf("a refusal changed the sheet: revision %d, spell %q", read.GetRevision(), entryOf(t, read, scrollThird).GetSpellKey())
	}
}

// A basic sheet has no items to put a spell on.
func TestSetScrollSpellOnABasicSheet(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	npc := et.master.create(t, et.campaign, charactersv1.CharacterKind_CHARACTER_KIND_MINION, "Capanga", basicSheet())
	_, err := et.setScroll(et.master, npc, "m1", "spell:fireball")
	wantEditorRefusal(t, "a basic sheet", err, invalidArg("character_id", "is a character with a basic sheet, which has no items to hold a scroll"))
}

// A plain save keeps the spell the sheet stores and drops the one the client sends, for an entry it keeps and for a new one.
func TestSaveKeepsTheScrollSpellAndDropsTheOneTheClientSends(t *testing.T) {
	t.Parallel()
	et, pc := scrollTable(t, scrollThird)
	id := entryOf(t, pc, scrollThird).GetId()
	pc, err := et.setScroll(et.master, pc, id, "spell:fireball")
	if err != nil {
		t.Fatal(err)
	}
	for who, u := range map[string]*user{"the owner": et.dona, "the master": et.master} {
		req := editorSaveOf(pc)
		req.Sheet = editorClone(pc.GetSheet(), func(f *charactersv1.FullSheet) {
			f.MagicItems[0].SpellKey = "spell:lightning-bolt" // the client says another spell
			f.MagicItems[0].SpellNamePt = "forjado"
			f.MagicItems[0].SpellGonePt, f.MagicItems[0].SpellOff = "forjado", true
			f.MagicItems = append(f.MagicItems, &charactersv1.SheetMagicItem{Key: scrollThird, SpellKey: "spell:fireball"}) // and one on a new entry
		})
		saved, err := editorUpdate(u, req)
		if err != nil {
			t.Fatalf("%s saves: %v", who, err)
		}
		before := len(pc.GetSheet().GetFull().GetMagicItems())
		var kept, fresh []string
		for _, it := range saved.GetSheet().GetFull().GetMagicItems() {
			if it.GetId() == id {
				kept = append(kept, it.GetSpellKey()+"/"+it.GetSpellNamePt()+"/"+it.GetSpellGonePt())
			} else {
				fresh = append(fresh, it.GetSpellKey())
			}
		}
		if len(kept) != 1 || kept[0] != "spell:fireball/Bola de Fogo/" || len(fresh) != before || slices.ContainsFunc(fresh, func(k string) bool { return k != "" }) {
			t.Errorf("%s: the saved entry reads %v and the others %v, want the stored spell and no spell on the %d others", who, kept, fresh, before)
		}
		if saved.GetSheet().GetFull().GetMagicItems()[0].GetSpellOff() {
			t.Errorf("%s: a forged spell_off was kept", who)
		}
		pc = et.read(t, et.master, pc.GetId())
	}
}

func scrollRead(t *testing.T, et *editorTable, u *user, id string) *charactersv1.SheetMagicItem {
	t.Helper()
	return entryOf(t, et.read(t, u, id), scrollThird)
}

// RN-23: a spell the master switches off or archives after it was set is never named to the owner, who reads the words in its
// place; the master reads the key and the name with the flag. A save of the owner keeps the stored key.
func TestTheScrollSpellTheMasterHidesIsNotNamedToTheOwner(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		spell func(et *editorTable) string
		hide  func(t *testing.T, et *editorTable, spell string)
		label string
	}{
		{
			"switched off", func(*editorTable) string { return "spell:fireball" },
			func(t *testing.T, et *editorTable, spell string) {
				t.Helper()
				et.master.setOff(t, et.campaign, true, spell)
			}, "Bola de Fogo",
		},
		{"archived", func(et *editorTable) string {
			return et.master.addEntry(t, et.campaign, choiceTableSpell(3, false, "class:wizard")).GetKey()
		}, func(t *testing.T, et *editorTable, spell string) {
			t.Helper()
			if _, err := et.master.table.ArchiveTableEntry(t.Context(), connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: et.campaign, Key: spell})); err != nil {
				t.Fatalf("ArchiveTableEntry: %v", err)
			}
		}, "Magia da mesa"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			et, pc := scrollTable(t, scrollThird)
			id := entryOf(t, pc, scrollThird).GetId()
			spell := c.spell(et)
			pc, err := et.setScroll(et.master, pc, id, spell)
			if err != nil {
				t.Fatalf("the master puts the spell on the scroll: %v", err)
			}
			// The positive control: while the spell is on, the owner reads its key and its name.
			if it := scrollRead(t, et, et.dona, pc.GetId()); it.GetSpellKey() != spell || it.GetSpellNamePt() != c.label || it.GetSpellGonePt() != "" || !it.GetConsumable().GetReadsScrollSpell() {
				t.Fatalf("the owner reads %q/%q/%q before the switch, want %q/%q", it.GetSpellKey(), it.GetSpellNamePt(), it.GetSpellGonePt(), spell, c.label)
			}
			c.hide(t, et, spell)

			owner := et.read(t, et.dona, pc.GetId())
			it := entryOf(t, owner, scrollThird)
			if it.GetSpellKey() != "" || it.GetSpellNamePt() != "" || it.GetSpellGonePt() != scrollGone || it.GetSpellOff() {
				t.Errorf("the owner reads key %q name %q words %q off %v, want only the words %q", it.GetSpellKey(), it.GetSpellNamePt(), it.GetSpellGonePt(), it.GetSpellOff(), scrollGone)
			}
			if cons := it.GetConsumable(); cons.GetKind() != charactersv1.ConsumableKind_CONSUMABLE_KIND_SCROLL || cons.GetReadsScrollSpell() || cons.GetScrollUnreadablePt() != "" || cons.GetScrollSpellLevel() != 0 || cons.GetScrollCheckNeeded() || cons.GetScrollCheckDc() != 0 {
				t.Errorf("the owner's consumable reads %v, want a scroll that says nothing of the spell", cons)
			}
			if text := owner.String(); strings.Contains(text, spell) || strings.Contains(text, c.label) {
				t.Errorf("the owner's character names the hidden spell: %s", text)
			}

			master := et.read(t, et.master, pc.GetId())
			mit := entryOf(t, master, scrollThird)
			if mit.GetSpellKey() != spell || mit.GetSpellNamePt() != c.label || !mit.GetSpellOff() || mit.GetSpellGonePt() != "" {
				t.Errorf("the master reads key %q name %q off %v words %q, want %q/%q with the flag and no words", mit.GetSpellKey(), mit.GetSpellNamePt(), mit.GetSpellOff(), mit.GetSpellGonePt(), spell, c.label)
			}

			// The owner's save, which sends no key, keeps the stored one.
			req := editorSaveOf(owner)
			req.Sheet = editorClone(owner.GetSheet(), func(f *charactersv1.FullSheet) { f.Languages = append(f.Languages, "Élfico") })
			saved, err := editorUpdate(et.dona, req)
			if err != nil {
				t.Fatalf("the owner saves: %v", err)
			}
			if it := entryOf(t, saved, scrollThird); it.GetSpellKey() != "" || it.GetSpellGonePt() != scrollGone || strings.Contains(saved.String(), spell) {
				t.Errorf("the owner's save answers %q/%q", it.GetSpellKey(), it.GetSpellGonePt())
			}
			if mit := scrollRead(t, et, et.master, pc.GetId()); mit.GetSpellKey() != spell || !mit.GetSpellOff() {
				t.Errorf("after the owner's save the master reads %q off %v, want %q kept", mit.GetSpellKey(), mit.GetSpellOff(), spell)
			}
		})
	}
}

// The master puts another spell on a scroll whose spell is gone, and the owner reads the new one.
func TestTheMasterReplacesAHiddenScrollSpell(t *testing.T) {
	t.Parallel()
	et, pc := scrollTable(t, scrollThird)
	id := entryOf(t, pc, scrollThird).GetId()
	pc, err := et.setScroll(et.master, pc, id, "spell:fireball")
	if err != nil {
		t.Fatal(err)
	}
	et.master.setOff(t, et.campaign, true, "spell:fireball")
	if _, err = et.setScroll(et.master, pc, id, "spell:lightning-bolt"); err != nil {
		t.Fatalf("the master puts another spell on the scroll: %v", err)
	}
	if it := scrollRead(t, et, et.dona, pc.GetId()); it.GetSpellKey() != "spell:lightning-bolt" || it.GetSpellGonePt() != "" {
		t.Errorf("the owner reads %q/%q", it.GetSpellKey(), it.GetSpellGonePt())
	}
}

// scrollNPC is an NPC the master made with the sheet and the scrolls, each with its spell.
func scrollNPC(t *testing.T, et *editorTable, sheet *charactersv1.CharacterSheet, spells map[string]string) *charactersv1.Character {
	t.Helper()
	npc := et.master.create(t, et.campaign, charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, "Leitor", sheet)
	req := editorSaveOf(npc)
	req.Sheet = editorClone(npc.GetSheet(), func(f *charactersv1.FullSheet) {
		for key := range spells {
			f.MagicItems = append(f.MagicItems, &charactersv1.SheetMagicItem{Key: key})
		}
	})
	npc, err := editorUpdate(et.master, req)
	if err != nil {
		t.Fatalf("the master puts the scrolls on the NPC: %v", err)
	}
	for key, spell := range spells {
		if npc, err = et.setScroll(et.master, npc, entryOf(t, npc, key).GetId(), spell); err != nil {
			t.Fatalf("the master puts %s on %s: %v", spell, key, err)
		}
	}
	return npc
}

func wantScrollFields(t *testing.T, who string, c *charactersv1.Character, key string, want *charactersv1.SheetConsumable) {
	t.Helper()
	got := entryOf(t, c, key).GetConsumable()
	if !proto.Equal(got, want) {
		t.Errorf("%s: %s reads consumable %v, want %v", who, key, got, want)
	}
}

func scroll(f func(*charactersv1.SheetConsumable)) *charactersv1.SheetConsumable {
	c := &charactersv1.SheetConsumable{Kind: charactersv1.ConsumableKind_CONSUMABLE_KIND_SCROLL}
	f(c)
	return c
}

// The scroll's entry says, for the reader, whether they read the spell, why not, whether a check is owed with its DC and
// ability, and what the casting costs: a wizard on and off the list and above the level they cast.
func TestSheetConsumableReadsTheScrollOfAWizard(t *testing.T) {
	t.Parallel()
	et, pc := scrollTable(t, scrollCantrip, scrollFirst, scrollSecond, scrollThird)
	for key, spell := range map[string]string{scrollCantrip: "spell:fire-bolt", scrollFirst: "spell:cure-wounds", scrollSecond: "spell:web", scrollThird: "spell:fireball"} {
		var err error
		if pc, err = et.setScroll(et.master, pc, entryOf(t, pc, key).GetId(), spell); err != nil {
			t.Fatalf("%s: %v", spell, err)
		}
	}
	action := rulesv1.ActionEconomy_ACTION_ECONOMY_ACTION
	for who, u := range map[string]*user{"the owner": et.dona, "the master": et.master} {
		read := et.read(t, u, pc.GetId())
		wantScrollFields(t, who, read, scrollCantrip, scroll(func(c *charactersv1.SheetConsumable) {
			c.ReadsScrollSpell, c.ScrollCastingEconomy = true, action
		}))
		wantScrollFields(t, who+", off the list", read, scrollFirst, scroll(func(c *charactersv1.SheetConsumable) {
			c.ScrollSpellLevel, c.ScrollUnreadablePt = 1, "Não é da lista de Mago."
		}))
		wantScrollFields(t, who+", a level they cast", read, scrollSecond, scroll(func(c *charactersv1.SheetConsumable) {
			c.ScrollSpellLevel, c.ReadsScrollSpell, c.ScrollCastingEconomy = 2, true, action
		}))
		wantScrollFields(t, who+", above the level they cast", read, scrollThird, scroll(func(c *charactersv1.SheetConsumable) {
			c.ScrollSpellLevel, c.ReadsScrollSpell, c.ScrollCastingEconomy = 3, true, action
			c.ScrollCheckNeeded, c.ScrollCheckDc, c.ScrollCheckAbility = true, 13, rulesv1.Ability_ABILITY_INTELLIGENCE
		}))
	}
}

// A scroll with no spell, and an item that is no scroll, carry no scroll field.
func TestSheetConsumableReadsNoScrollFieldWithoutASpell(t *testing.T) {
	t.Parallel()
	et, pc := scrollTable(t, scrollThird, potionKey, "item:bag-of-holding")
	for who, u := range map[string]*user{"the owner": et.dona, "the master": et.master} {
		read := et.read(t, u, pc.GetId())
		wantScrollFields(t, who+", no spell", read, scrollThird, scroll(func(*charactersv1.SheetConsumable) {}))
		wantScrollFields(t, who+", a potion", read, potionKey, &charactersv1.SheetConsumable{Kind: charactersv1.ConsumableKind_CONSUMABLE_KIND_POTION, CostsAction: true, HealDiceCount: 2, HealDiceSides: 4, HealBonus: 2})
		if got := entryOf(t, read, "item:bag-of-holding").GetConsumable(); got != nil {
			t.Errorf("%s: a bag reads consumable %v", who, got)
		}
	}
}

// A warlock casts with pact slots only: its level counts as the highest it casts, and a reaction spell costs the reaction.
func TestSheetConsumableCountsThePactSlotsOfAWarlock(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	pc := scrollNPC(t, et, choiceSheet("race:human", "", "class:warlock", "subclass:fiend", 1), map[string]string{scrollFirst: "spell:hellish-rebuke", scrollSecond: "spell:hold-person"})
	read := et.read(t, et.master, pc.GetId())
	wantScrollFields(t, "the warlock, the pact slot level", read, scrollFirst, scroll(func(c *charactersv1.SheetConsumable) {
		c.ScrollSpellLevel, c.ReadsScrollSpell, c.ScrollCastingEconomy = 1, true, rulesv1.ActionEconomy_ACTION_ECONOMY_REACTION
	}))
	wantScrollFields(t, "the warlock, above the pact slot", read, scrollSecond, scroll(func(c *charactersv1.SheetConsumable) {
		c.ScrollSpellLevel, c.ReadsScrollSpell, c.ScrollCastingEconomy = 2, true, rulesv1.ActionEconomy_ACTION_ECONOMY_ACTION
		c.ScrollCheckNeeded, c.ScrollCheckDc, c.ScrollCheckAbility = true, 12, rulesv1.Ability_ABILITY_CHARISMA
	}))
}

// A character with no spellcasting does not read the spell, and is told so.
func TestSheetConsumableOfAReaderWhoDoesNotCast(t *testing.T) {
	t.Parallel()
	et := newEditorTable(t, nil)
	pc := scrollNPC(t, et, enemySheet(), map[string]string{scrollFirst: "spell:magic-missile"})
	wantScrollFields(t, "the fighter", et.read(t, et.master, pc.GetId()), scrollFirst, scroll(func(c *charactersv1.SheetConsumable) {
		c.ScrollSpellLevel, c.ScrollUnreadablePt = 1, "Não conjura magias."
	}))
}
