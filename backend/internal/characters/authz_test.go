package characters

import (
	"context"
	"errors"
	"strings"
	"testing"
	"uuid"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
)

// allowed marks a call that must succeed in the authorization matrix.
const allowed connect.Code = 0

// TestAuthorizationMatrix calls every CharacterService and ContentService
// method as each kind of caller and checks who gets in (ADR-0011, and the
// table in docs/architecture/README.md):
//
//	master        the campaign's master
//	owner         the player who owns the character the call is about
//	other player  another player of the same campaign
//	non-member    signed in, but not in the campaign: not_found, so the
//	              campaign's existence does not leak
//	anonymous     unauthenticated, always
//	pending       a pending member (RN-15, MR-024) with their own pending
//	              character: treated as a player in the calls package authz
//	              lets them make (pendingMayCall), where they see only that
//	              character; not_found everywhere else, like a non-member
//
// A member who may not see a character gets not_found too, so NPC IDs and
// other players' characters do not leak; a member asking for something only
// the master may do gets permission_denied.
func TestAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, owner, other := h.newUser("Mestre"), h.newUser("Dona"), h.newUser("Outra")
	campaign := h.newCampaign(master, "Mirathel", owner, other)
	pc := owner.createPensantus(t, campaign)
	npc := master.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_BOSS, "Strahd", enemySheet())
	// Three pending members (RN-15): the "pending" caller, and two whose
	// characters the ApproveCharacter and RejectCharacter rows decide on,
	// so those rows do not change the pending caller.
	pending, approvee, rejectee := h.newUser("Pendente"), h.newUser("Aprovada"), h.newUser("Recusado")
	h.joinPending(master, campaign, pending, approvee, rejectee)
	pendingPC := pending.createPensantus(t, campaign)
	approveePC := approvee.createPensantus(t, campaign)
	rejecteePC := rejectee.createPensantus(t, campaign)
	// Two reserved characters (MR-049): the rows of the claim links and of "Excluir".
	reserved := master.reservedByMaster(t, campaign, "Reservado")
	toDelete := master.reservedByMaster(t, campaign, "Para apagar")

	// fresh reads a character's current revision as the master, so every
	// caller's write is judged on its permission, not on a stale revision.
	fresh := func(id string) *charactersv1.Character { return master.get(t, campaign, id) }
	// gift is a creature the master gave Pensantus, for the creature rows.
	var giftCreature *charactersv1.CharacterCreature
	gift := func() *charactersv1.CharacterCreature {
		if giftCreature == nil {
			res, err := master.api.GiveCreature(t.Context(), connect.NewRequest(&charactersv1.GiveCreatureRequest{CampaignId: campaign, CharacterId: pc.GetId(), MonsterKey: "monster:wolf", Name: "Presa"}))
			if err != nil {
				t.Fatalf("GiveCreature() error = %v", err)
			}
			giftCreature = res.Msg.GetCreature()
		}
		return giftCreature
	}
	create := func(kind charactersv1.CharacterKind, sheet *charactersv1.CharacterSheet) func(ctx context.Context, u *user) error {
		return func(ctx context.Context, u *user) error {
			_, err := u.api.CreateCharacter(ctx, connect.NewRequest(&charactersv1.CreateCharacterRequest{
				CampaignId: campaign, Kind: kind, Name: "Novo", Sheet: sheet,
			}))
			return err
		}
	}
	get := func(id string) func(ctx context.Context, u *user) error {
		return func(ctx context.Context, u *user) error {
			_, err := u.api.GetCharacter(ctx, connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: campaign, CharacterId: id}))
			return err
		}
	}
	update := func(id string) func(ctx context.Context, u *user) error {
		return func(ctx context.Context, u *user) error {
			c := fresh(id)
			_, err := u.api.UpdateCharacter(ctx, connect.NewRequest(&charactersv1.UpdateCharacterRequest{
				CampaignId: campaign, CharacterId: id, Revision: c.GetRevision(), Name: c.GetName(), Sheet: c.GetSheet(),
			}))
			return err
		}
	}
	updateStory := func(id string) func(ctx context.Context, u *user) error {
		return func(ctx context.Context, u *user) error {
			_, err := u.api.UpdateCharacterStory(ctx, connect.NewRequest(&charactersv1.UpdateCharacterStoryRequest{
				CampaignId: campaign, CharacterId: id, Revision: fresh(id).GetRevision(), Story: &charactersv1.CharacterStory{Allies: "A Ordem"},
			}))
			return err
		}
	}

	type rpc = func(ctx context.Context, u *user) error
	rows := []struct {
		method string // the RPC, to check that every method has a row
		label  string
		before func() // runs once, before the row's calls
		call   rpc
		// The expected code for master, owner, other player, non-member,
		// anonymous, pending member.
		want [6]connect.Code
	}{
		// The owner already has a living character (RN-03); the other
		// player has none yet, so they may create theirs.
		{
			"CreateCharacter", "player character", nil, create(charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, pensantusSheet()),
			[6]connect.Code{connect.CodePermissionDenied, connect.CodeFailedPrecondition, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeFailedPrecondition},
		},
		{
			"CreateCharacter", "enemy", nil, create(charactersv1.CharacterKind_CHARACTER_KIND_ENEMY, enemySheet()),
			[6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodePermissionDenied},
		},
		{
			"CreateCharacter", "minion", nil, create(charactersv1.CharacterKind_CHARACTER_KIND_MINION, basicSheet()),
			[6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodePermissionDenied},
		},

		// "Criar NPC" from a creature (MR-042): the master's, like any NPC.
		{
			"CreateNpcFromCreature", "", nil, func(ctx context.Context, u *user) error {
				_, err := u.api.CreateNpcFromCreature(ctx, connect.NewRequest(&charactersv1.CreateNpcFromCreatureRequest{
					CampaignId: campaign, CreatureKey: "monster:ogre", Name: "Grak", Kind: charactersv1.CharacterKind_CHARACTER_KIND_MINION,
					IdempotencyKey: uuid.New().String(),
				}))
				return err
			},
			[6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound},
		},

		{
			"GetCharacter", "player character", nil, get(pc.GetId()),
			[6]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound},
		},
		{
			"GetCharacter", "NPC", nil, get(npc.GetId()),
			[6]connect.Code{allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound},
		},

		{"ListCharacters", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.ListCharacters(ctx, connect.NewRequest(&charactersv1.ListCharactersRequest{CampaignId: campaign}))
			return err
		}, [6]connect.Code{allowed, allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, allowed}},

		// The editor's preview answers what the save would (RN-15 included): a new PLAYER sheet
		// is the player's or the pending member's, and an existing character is read and
		// edited by whoever sees it and may edit its sheet.
		{"PreviewCharacter", "new player sheet", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.PreviewCharacter(ctx, connect.NewRequest(&charactersv1.PreviewCharacterRequest{CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: pensantusSheet()}))
			return err
		}, [6]connect.Code{connect.CodePermissionDenied, allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, allowed}},
		{"PreviewCharacter", "character", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.PreviewCharacter(ctx, connect.NewRequest(&charactersv1.PreviewCharacterRequest{CampaignId: campaign, CharacterId: pc.GetId(), Sheet: pensantusSheet()}))
			return err
		}, [6]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"PreviewCharacter", "pending character", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.PreviewCharacter(ctx, connect.NewRequest(&charactersv1.PreviewCharacterRequest{CampaignId: campaign, CharacterId: pendingPC.GetId(), Sheet: pensantusSheet()}))
			return err
		}, [6]connect.Code{allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, allowed}},
		// PreviewChoices follows PreviewCharacter: the same callers and the same sheets,
		// and, for a character, whoever sees it.
		{"PreviewChoices", "new player sheet", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.PreviewChoices(ctx, connect.NewRequest(&charactersv1.PreviewChoicesRequest{CampaignId: campaign, Kind: charactersv1.CharacterKind_CHARACTER_KIND_PLAYER, Sheet: pensantusSheet()}))
			return err
		}, [6]connect.Code{connect.CodePermissionDenied, allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, allowed}},
		{"PreviewChoices", "character", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.PreviewChoices(ctx, connect.NewRequest(&charactersv1.PreviewChoicesRequest{CampaignId: campaign, CharacterId: pc.GetId(), Sheet: pensantusSheet()}))
			return err
		}, [6]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"PreviewChoices", "pending character", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.PreviewChoices(ctx, connect.NewRequest(&charactersv1.PreviewChoicesRequest{CampaignId: campaign, CharacterId: pendingPC.GetId()}))
			return err
		}, [6]connect.Code{allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, allowed}},
		// Only the master reads who has choices open; a player and a stranger are told
		// the campaign does not exist.
		{"GetCampaignOpenChoices", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.GetCampaignOpenChoices(ctx, connect.NewRequest(&charactersv1.GetCampaignOpenChoicesRequest{CampaignId: campaign}))
			return err
		}, [6]connect.Code{allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{
			"UpdateCharacter", "draft", nil, update(pc.GetId()),
			[6]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound},
		},
		{
			"UpdateCharacter", "NPC", nil, update(npc.GetId()),
			[6]connect.Code{allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound},
		},

		{
			"UpdateCharacterStory", "draft", nil, updateStory(pc.GetId()),
			[6]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound},
		},
		{
			"UpdateCharacterStory", "NPC", nil, updateStory(npc.GetId()),
			[6]connect.Code{allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound},
		},

		// The pending member's own pending character (RN-15): they read and
		// edit it; nobody but the master sees it.
		{
			"GetCharacter", "pending character", nil, get(pendingPC.GetId()),
			[6]connect.Code{allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, allowed},
		},
		{
			"UpdateCharacter", "pending character", nil, update(pendingPC.GetId()),
			[6]connect.Code{allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, allowed},
		},
		{
			"UpdateCharacterStory", "pending character", nil, updateStory(pendingPC.GetId()),
			[6]connect.Code{allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, allowed},
		},

		// Only the master settles a pending character. Each row decides on
		// another pending member's character, so the pending caller stays
		// pending for the rows below.
		{"ApproveCharacter", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.ApproveCharacter(ctx, connect.NewRequest(&charactersv1.ApproveCharacterRequest{CampaignId: campaign, CharacterId: approveePC.GetId()}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"RejectCharacter", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.RejectCharacter(ctx, connect.NewRequest(&charactersv1.RejectCharacterRequest{CampaignId: campaign, CharacterId: rejecteePC.GetId()}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		// "Pedir ajustes": the master asks, the owner sends it again. The pending caller owns
		// pendingPC, so the master's row opens the request the owner's row answers. Whoever is
		// not the owner gets the answer of a character that is not there.
		{"RequestCharacterChanges", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.RequestCharacterChanges(ctx, connect.NewRequest(&charactersv1.RequestCharacterChangesRequest{
				CampaignId: campaign, CharacterId: pendingPC.GetId(), Reason: "Falta o equipamento.", IdempotencyKey: uuid.New().String(),
			}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"ResubmitCharacter", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.ResubmitCharacter(ctx, connect.NewRequest(&charactersv1.ResubmitCharacterRequest{
				CampaignId: campaign, CharacterId: pendingPC.GetId(), IdempotencyKey: uuid.New().String(),
			}))
			return err
		}, [6]connect.Code{connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, allowed}},

		{"SetStoryEditing", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.SetStoryEditing(ctx, connect.NewRequest(&charactersv1.SetStoryEditingRequest{CampaignId: campaign, CharacterId: pc.GetId(), Allowed: true}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		{"GetMasterNotes", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.GetMasterNotes(ctx, connect.NewRequest(&charactersv1.GetMasterNotesRequest{CampaignId: campaign, CharacterId: pc.GetId()}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		{"UpdateMasterNotes", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.UpdateMasterNotes(ctx, connect.NewRequest(&charactersv1.UpdateMasterNotesRequest{CampaignId: campaign, CharacterId: pc.GetId(), Notes: "segredo"}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		{"ListContent", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.content.ListContent(ctx, connect.NewRequest(&rulesv1.ListContentRequest{CampaignId: campaign}))
			return err
		}, [6]connect.Code{allowed, allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, allowed}},

		{"GetSpellDetails", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.content.GetSpellDetails(ctx, connect.NewRequest(&rulesv1.GetSpellDetailsRequest{CampaignId: campaign, SpellKey: "spell:fire-bolt"}))
			return err
		}, [6]connect.Code{allowed, allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, allowed}},

		// The "Magias" page (MR-045) is for an active member: the pending member
		// has the catalog (ListContent) and gets not_found here (RN-15).
		{"ListSpells", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.content.ListSpells(ctx, connect.NewRequest(&rulesv1.ListSpellsRequest{CampaignId: campaign, Query: "mãos"}))
			return err
		}, [6]connect.Code{allowed, allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		// The magic item picker is for an active member, like the spells.
		{"ListMagicItems", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.content.ListMagicItems(ctx, connect.NewRequest(&rulesv1.ListMagicItemsRequest{CampaignId: campaign, Query: "anel"}))
			return err
		}, [6]connect.Code{allowed, allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		// The creatures are public rules, but only for an active member: the
		// pending member gets not_found (RN-15).
		{"ListCreatures", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.content.ListCreatures(ctx, connect.NewRequest(&rulesv1.ListCreaturesRequest{CampaignId: campaign, Query: "lobo"}))
			return err
		}, [6]connect.Code{allowed, allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		{"GetCreature", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.content.GetCreature(ctx, connect.NewRequest(&rulesv1.GetCreatureRequest{CampaignId: campaign, Key: "monster:wolf"}))
			return err
		}, [6]connect.Code{allowed, allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		// The trap and light presets are public rules too, for active members only.
		{"ListTrapPresets", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.content.ListTrapPresets(ctx, connect.NewRequest(&rulesv1.ListTrapPresetsRequest{CampaignId: campaign}))
			return err
		}, [6]connect.Code{allowed, allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		{"ListLightPresets", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.content.ListLightPresets(ctx, connect.NewRequest(&rulesv1.ListLightPresetsRequest{CampaignId: campaign}))
			return err
		}, [6]connect.Code{allowed, allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		// The table's own content (RN-23): the master writes, every active member
		// reads, a pending member only reads the catalog (ListContent).
		{"CreateTableEntry", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.table.CreateTableEntry(ctx, connect.NewRequest(&rulesv1.CreateTableEntryRequest{
				CampaignId: campaign, Body: &rulesv1.CreateTableEntryRequest_TableBackground{TableBackground: testBackground("Guarda de farol")},
			}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		{"ListTableEntries", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.table.ListTableEntries(ctx, connect.NewRequest(&rulesv1.ListTableEntriesRequest{CampaignId: campaign}))
			return err
		}, [6]connect.Code{allowed, allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		{"UpdateTableEntry", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.table.UpdateTableEntry(ctx, connect.NewRequest(&rulesv1.UpdateTableEntryRequest{
				CampaignId: campaign, Key: "background:guarda-de-farol@mesa", ExpectedRevision: tableRevisionOf(t, master, campaign, "background:guarda-de-farol@mesa"),
				Body: &rulesv1.UpdateTableEntryRequest_TableBackground{TableBackground: testBackground("Guarda de farol")},
			}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		{"ArchiveTableEntry", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.table.ArchiveTableEntry(ctx, connect.NewRequest(&rulesv1.ArchiveTableEntryRequest{CampaignId: campaign, Key: "background:guarda-de-farol@mesa"}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		{"UnarchiveTableEntry", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.table.UnarchiveTableEntry(ctx, connect.NewRequest(&rulesv1.UnarchiveTableEntryRequest{CampaignId: campaign, Key: "background:guarda-de-farol@mesa"}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		{"ExportTableContent", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.table.ExportTableContent(ctx, connect.NewRequest(&rulesv1.ExportTableContentRequest{CampaignId: campaign}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		{"ImportTableContent", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.table.ImportTableContent(ctx, connect.NewRequest(&rulesv1.ImportTableContentRequest{
				CampaignId: campaign, Mode: rulesv1.TableImportMode_TABLE_IMPORT_MODE_PREVIEW,
				Pack: &rulesv1.TableContentPack{Format: "meurpg.table-content", Version: 1},
			}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		{"ListOptionSwitches", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.table.ListOptionSwitches(ctx, connect.NewRequest(&rulesv1.ListOptionSwitchesRequest{CampaignId: campaign}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		{"SetOptionSwitches", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.table.SetOptionSwitches(ctx, connect.NewRequest(switchReq(campaign, false, "class:wizard")))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		{"GetClassTableDefaults", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.table.GetClassTableDefaults(ctx, connect.NewRequest(&rulesv1.GetClassTableDefaultsRequest{CampaignId: campaign}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		{"GetEffectMenu", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.table.GetEffectMenu(ctx, connect.NewRequest(&rulesv1.GetEffectMenuRequest{CampaignId: campaign}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		// A game session starts: the sheet locks, and the story permission
		// the master gave above ends (RN-01).
		{
			"UpdateCharacter", "locked", func() { h.lockSheets(campaign) }, update(pc.GetId()),
			[6]connect.Code{allowed, connect.CodeFailedPrecondition, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound},
		},
		{"PreviewCharacter", "locked character", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.PreviewCharacter(ctx, connect.NewRequest(&charactersv1.PreviewCharacterRequest{CampaignId: campaign, CharacterId: pc.GetId(), Sheet: pensantusSheet()}))
			return err
		}, [6]connect.Code{allowed, connect.CodeFailedPrecondition, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{
			"UpdateCharacterStory", "locked", nil, updateStory(pc.GetId()),
			[6]connect.Code{allowed, connect.CodeFailedPrecondition, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound},
		},

		// The guided level-up (MR-040): the sheet is locked now, the character
		// has the XP for level 4, and the XP rule is wired. The master and the
		// owner read the options; only the owner rolls and levels up (the
		// master keeps the editor); only the master reads the record.
		{"GetLevelUpOptions", "", func() {
			h.svc.SetLevelUps(xpLevelUps{})
			c := fresh(pc.GetId())
			sheet := proto.CloneOf(c.GetSheet())
			sheet.GetFull().ExperiencePoints = 2700
			if _, err := master.update(t, c, c.GetName(), sheet); err != nil {
				t.Fatalf("give the XP: %v", err)
			}
		}, func(ctx context.Context, u *user) error {
			_, err := u.api.GetLevelUpOptions(ctx, connect.NewRequest(&charactersv1.GetLevelUpOptionsRequest{CampaignId: campaign, CharacterId: pc.GetId()}))
			return err
		}, [6]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"PreviewLevelUp", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.PreviewLevelUp(ctx, connect.NewRequest(&charactersv1.PreviewLevelUpRequest{CampaignId: campaign, CharacterId: pc.GetId(), Choices: pensantusLevelUp()}))
			return err
		}, [6]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"RollLevelUpHitPoints", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.RollLevelUpHitPoints(ctx, connect.NewRequest(&charactersv1.RollLevelUpHitPointsRequest{CampaignId: campaign, CharacterId: pc.GetId(), IdempotencyKey: uuid.New().String()}))
			return err
		}, [6]connect.Code{connect.CodePermissionDenied, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		// The ability scores of a new sheet (RN-24): any player may roll and read their
		// own 4d6, a pending member too (they create their character); the master has
		// none to make. The roll is idempotent, so it can sit anywhere in the table.
		{"GetAbilityRolls", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.GetAbilityRolls(ctx, connect.NewRequest(&charactersv1.GetAbilityRollsRequest{CampaignId: campaign}))
			return err
		}, [6]connect.Code{connect.CodePermissionDenied, allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, allowed}},
		{"RollAbilityScores", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.RollAbilityScores(ctx, connect.NewRequest(&charactersv1.RollAbilityScoresRequest{CampaignId: campaign}))
			return err
		}, [6]connect.Code{connect.CodePermissionDenied, allowed, allowed, connect.CodeNotFound, connect.CodeUnauthenticated, allowed}},
		{"ListLevelUps", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.ListLevelUps(ctx, connect.NewRequest(&charactersv1.ListLevelUpsRequest{CampaignId: campaign}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		// The owner's call goes through, so it comes after the reads above.
		{"LevelUpCharacter", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.LevelUpCharacter(ctx, connect.NewRequest(&charactersv1.LevelUpCharacterRequest{
				CampaignId: campaign, CharacterId: pc.GetId(), Revision: fresh(pc.GetId()).GetRevision(), Choices: pensantusLevelUp(),
			}))
			return err
		}, [6]connect.Code{connect.CodePermissionDenied, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		// The character's creatures (MR-037): the master and the owner's player
		// read them; every other player and the pending member get not_found, as
		// for the character itself; only the master gives and corrects hit points.
		{"GiveCreature", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.GiveCreature(ctx, connect.NewRequest(&charactersv1.GiveCreatureRequest{CampaignId: campaign, CharacterId: pc.GetId(), MonsterKey: "monster:wolf"}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"ListCharacterCreatures", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.ListCharacterCreatures(ctx, connect.NewRequest(&charactersv1.ListCharacterCreaturesRequest{CampaignId: campaign, CharacterId: pc.GetId()}))
			return err
		}, [6]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		// What the character can summon (MR-037): the same access as the list.
		{"GetSummonOptions", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.GetSummonOptions(ctx, connect.NewRequest(&charactersv1.GetSummonOptionsRequest{CampaignId: campaign, CharacterId: pc.GetId()}))
			return err
		}, [6]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		// The beasts a druid may become (MR-037): the master and the character's own
		// player read them; another player and the pending member get not_found.
		{"ListWildShapeForms", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.ListWildShapeForms(ctx, connect.NewRequest(&charactersv1.ListWildShapeFormsRequest{CampaignId: campaign, CharacterId: pc.GetId()}))
			return err
		}, [6]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"RenameCreature", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.RenameCreature(ctx, connect.NewRequest(&charactersv1.RenameCreatureRequest{CampaignId: campaign, CreatureId: gift().GetId(), Name: "Presa"}))
			return err
		}, [6]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"AdjustCreatureHitPoints", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.AdjustCreatureHitPoints(ctx, connect.NewRequest(&charactersv1.AdjustCreatureHitPointsRequest{
				CampaignId: campaign, CreatureId: gift().GetId(), Change: &charactersv1.AdjustCreatureHitPointsRequest_Damage{Damage: 1},
			}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		// The master dismisses it; for the owner it is gone already, which changes
		// nothing; another player never sees it.
		{"DismissCreature", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.DismissCreature(ctx, connect.NewRequest(&charactersv1.DismissCreatureRequest{CampaignId: campaign, CreatureId: gift().GetId()}))
			return err
		}, [6]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		// A game session does not lock a pending character (RN-15): its
		// player still edits it.
		{
			"UpdateCharacter", "pending character, after a session started", nil, update(pendingPC.GetId()),
			[6]connect.Code{allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, allowed},
		},
		// A pending character is approved or rejected, never marked dead.
		{"MarkCharacterDead", "pending character", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.MarkCharacterDead(ctx, connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: campaign, CharacterId: pendingPC.GetId()}))
			return err
		}, [6]connect.Code{connect.CodeFailedPrecondition, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		// The wizard of the table has nothing open: the callers that may complete choices
		// reach the rule that says so, the others are told the character is not theirs.
		{"CompleteCharacterChoices", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.CompleteCharacterChoices(ctx, connect.NewRequest(&charactersv1.CompleteCharacterChoicesRequest{
				CampaignId: campaign, CharacterId: pc.GetId(), ExpectedRevision: 1,
				Picks: []*charactersv1.ChoicePick{{ChoiceKey: "feature:fighter-fighting-style", OptionKeys: []string{"feature:fighter-fighting-style-defense"}}},
			}))
			return err
		}, [6]connect.Code{connect.CodeAborted, connect.CodeAborted, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		// Stowing a weapon: the master and the owner may (the second call finds it stowed already);
		// another player, a stranger and a pending member are told the character is not there.
		{"SetEquipped", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.SetEquipped(ctx, connect.NewRequest(&charactersv1.SetEquippedRequest{CampaignId: campaign, CharacterId: pc.GetId(), Key: "equipment:quarterstaff"}))
			return err
		}, [6]connect.Code{allowed, allowed, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		// Attuning an item: the master and the owner reach the rule (the key is no item of the sheet, so both
		// are told so); another player, a stranger and a pending member are told the character is not there.
		{"SetAttunement", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.SetAttunement(ctx, connect.NewRequest(&charactersv1.SetAttunementRequest{CampaignId: campaign, CharacterId: pc.GetId(), EntryId: "m99", Attuned: true}))
			return err
		}, [6]connect.Code{connect.CodeInvalidArgument, connect.CodeInvalidArgument, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		// A daily choice's pick: the master and the owner reach the rule (the sheet has no such choice, so both are told so);
		// another player, a stranger and a pending member are told the character is not there.
		{"SetDailyChoice", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.SetDailyChoice(ctx, connect.NewRequest(&charactersv1.SetDailyChoiceRequest{CampaignId: campaign, CharacterId: pc.GetId(), ChoiceId: "feat:nada#0", Key: "skill:stealth"}))
			return err
		}, [6]connect.Code{connect.CodeInvalidArgument, connect.CodeInvalidArgument, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		// Reserved characters and claim links (MR-049): only the master makes, revokes and gives
		// back; every other member is refused with the master's answer (permission_denied), and
		// whoever is no member, or only pending, cannot tell the campaign from one that does not exist.
		{"CreateClaimLink", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.CreateClaimLink(ctx, connect.NewRequest(&charactersv1.CreateClaimLinkRequest{CampaignId: campaign, CharacterId: reserved.GetId()}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"RevokeClaimLink", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.RevokeClaimLink(ctx, connect.NewRequest(&charactersv1.RevokeClaimLinkRequest{CampaignId: campaign, CharacterId: reserved.GetId()}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		// A reserved character has no player to take back: the master is refused for the state, the rest for the role.
		{"ReturnCharacterToReserve", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.ReturnCharacterToReserve(ctx, connect.NewRequest(&charactersv1.ReturnCharacterToReserveRequest{CampaignId: campaign, CharacterId: reserved.GetId()}))
			return err
		}, [6]connect.Code{connect.CodeFailedPrecondition, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		// The links answer every signed-in person with the same refusal for a link that does not work.
		{"PreviewClaim", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.PreviewClaim(ctx, connect.NewRequest(&charactersv1.PreviewClaimRequest{Token: "x"}))
			return err
		}, [6]connect.Code{connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"ClaimCharacter", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.ClaimCharacter(ctx, connect.NewRequest(&charactersv1.ClaimCharacterRequest{Token: "x"}))
			return err
		}, [6]connect.Code{connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
		{"DeleteReservedCharacter", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.DeleteReservedCharacter(ctx, connect.NewRequest(&charactersv1.DeleteReservedCharacterRequest{CampaignId: campaign, CharacterId: toDelete.GetId()}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		// Last, because it changes the owner's character for good.
		{"MarkCharacterDead", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.MarkCharacterDead(ctx, connect.NewRequest(&charactersv1.MarkCharacterDeadRequest{CampaignId: campaign, CharacterId: pc.GetId()}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},

		// "Reviver" brings the character the row above killed back: the master's alone.
		{"ReviveCharacter", "", nil, func(ctx context.Context, u *user) error {
			_, err := u.api.ReviveCharacter(ctx, connect.NewRequest(&charactersv1.ReviveCharacterRequest{
				CampaignId: campaign, CharacterId: pc.GetId(), IdempotencyKey: uuid.New().String(),
			}))
			return err
		}, [6]connect.Code{allowed, connect.CodePermissionDenied, connect.CodePermissionDenied, connect.CodeNotFound, connect.CodeUnauthenticated, connect.CodeNotFound}},
	}

	// Every method of both services must be in the table, so a new RPC
	// cannot ship without its authorization test.
	covered := map[string]bool{}
	for _, r := range rows {
		covered[r.method] = true
	}
	for _, service := range []protoreflect.ServiceDescriptor{
		charactersv1.File_meurpg_characters_v1_characters_proto.Services().ByName("CharacterService"),
		rulesv1.File_meurpg_rules_v1_rules_proto.Services().ByName("ContentService"),
		rulesv1.File_meurpg_rules_v1_table_content_proto.Services().ByName("TableContentService"),
	} {
		for i := range service.Methods().Len() {
			if name := string(service.Methods().Get(i).Name()); !covered[name] {
				t.Errorf("%s.%s is missing from the authorization matrix", service.Name(), name)
			}
		}
	}

	callers := []struct {
		name string
		user *user
	}{
		{"master", master},
		{"owner", owner},
		{"other player", other},
		{"non-member", h.newUser("De fora")},
		{"anonymous", h.anonymous()},
		{"pending", pending},
	}
	// Sequential on purpose: some rows change the state the next ones see.
	for _, r := range rows {
		if r.before != nil {
			r.before()
		}
		for i, caller := range callers {
			t.Run(strings.TrimSuffix(r.method+"/"+r.label, "/")+"/"+caller.name, func(t *testing.T) {
				err := r.call(t.Context(), caller.user)
				want := r.want[i]
				switch {
				case want == allowed && err != nil:
					t.Errorf("error = %v, want allowed", err)
				case want != allowed && connect.CodeOf(err) != want:
					t.Errorf("error = %v, want %v", err, want)
				}
			})
		}
	}
}

// TestNonMembersCannotTellCharactersApart: a player asking for a character
// they may not see gets the same answer as for one that does not exist,
// whatever the reason; so does a non-member for any character of the
// campaign.
func TestNonMembersCannotTellCharactersApart(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, owner, other, outsider := h.newUser("Mestre"), h.newUser("Dona"), h.newUser("Outra"), h.newUser("De fora")
	campaign := h.newCampaign(master, "Mirathel", owner, other)
	elsewhere := h.newCampaign(master, "Outra campanha", other)
	pc := owner.createPensantus(t, campaign)
	npc := master.create(t, campaign, charactersv1.CharacterKind_CHARACTER_KIND_STORY, "Taverneiro", basicSheet())
	otherCampaignPC := other.createPensantus(t, elsewhere)

	ids := []string{pc.GetId(), npc.GetId(), otherCampaignPC.GetId(), "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e0ff", "pensantus"}
	var messages []string
	for _, id := range ids {
		_, err := other.api.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: campaign, CharacterId: id}))
		wantCode(t, "other player's GetCharacter("+id+")", err, connect.CodeNotFound)
		messages = append(messages, err.Error())
		_, err = other.api.UpdateCharacterStory(t.Context(), connect.NewRequest(&charactersv1.UpdateCharacterStoryRequest{CampaignId: campaign, CharacterId: id, Revision: 1}))
		wantCode(t, "other player's UpdateCharacterStory("+id+")", err, connect.CodeNotFound)
		messages = append(messages, err.Error())
	}
	for _, msg := range messages[1:] {
		if msg != messages[0] {
			t.Errorf("error messages differ: %q", messages)
			break
		}
	}

	// A non-member cannot tell the campaign apart from one that does not
	// exist either.
	_, errReal := outsider.api.ListCharacters(t.Context(), connect.NewRequest(&charactersv1.ListCharactersRequest{CampaignId: campaign}))
	_, errFake := outsider.api.ListCharacters(t.Context(), connect.NewRequest(&charactersv1.ListCharactersRequest{CampaignId: "6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e0ff"}))
	if connect.CodeOf(errReal) != connect.CodeNotFound || errReal.Error() != errFake.Error() {
		t.Errorf("non-member errors differ: %v vs %v", errReal, errFake)
	}
}

// TestResponsesAreNotCached: every answer describes the caller's
// characters, so none may be stored by a browser or a proxy.
func TestResponsesAreNotCached(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	master, player := h.newUser("Mestre"), h.newUser("Jogadora")
	campaign := h.newCampaign(master, "Mirathel", player)
	c := player.createPensantus(t, campaign)

	res, err := player.api.GetCharacter(t.Context(), connect.NewRequest(&charactersv1.GetCharacterRequest{CampaignId: campaign, CharacterId: c.GetId()}))
	if err != nil {
		t.Fatalf("GetCharacter() error = %v", err)
	}
	if got := res.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("GetCharacter Cache-Control = %q, want no-store", got)
	}
	content, err := player.content.ListContent(t.Context(), connect.NewRequest(&rulesv1.ListContentRequest{CampaignId: campaign}))
	if err != nil {
		t.Fatalf("ListContent() error = %v", err)
	}
	if got := content.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("ListContent Cache-Control = %q, want no-store", got)
	}

	_, err = player.api.GetMasterNotes(t.Context(), connect.NewRequest(&charactersv1.GetMasterNotesRequest{CampaignId: campaign, CharacterId: c.GetId()}))
	ce, ok := errors.AsType[*connect.Error](err)
	if !ok || ce.Meta().Get("Cache-Control") != "no-store" {
		t.Errorf("error response %v lacks Cache-Control: no-store", err)
	}
}
