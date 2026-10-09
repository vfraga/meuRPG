package leaktest

import (
	"google.golang.org/protobuf/proto"

	campaignsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1/campaignsv1connect"
	charactersv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	mapsv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	notesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/notes/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/notes/v1/notesv1connect"
	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	progressionv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1/progressionv1connect"
	rulesv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
)

// reads is the table of every read a player (or a pending member, or a stranger) may
// try, one row for each call that matters. Each row is asked of everyone in the
// fixture (see read for the fields). A read that only the master may make is a row too,
// with masterOnlyRead: the table then proves that the others are refused with an answer
// that holds nothing. A procedure that is not a read is not here: it is classified in
// classify_test.go, and the guard (guard_test.go) fails when a procedure is in neither.
//
// To add a read: add a row. If the answer of the new read may hold something the master
// hid, the fixture (world_test.go) needs that thing, with a marker.
var reads = []read{
	// ===== CampaignService
	{
		procedure: campaignsv1connect.CampaignServiceGetCampaignProcedure, allow: membersAndPending, why: "a pending member reads the campaign's id, name and their own state (RN-15)",
		req: func(w *world) proto.Message {
			return &campaignsv1.GetCampaignRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: campaignsv1connect.CampaignServiceListMembersProcedure, allow: members,
		req: func(w *world) proto.Message {
			return &campaignsv1.ListMembersRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: campaignsv1connect.CampaignServiceListMyCampaignsProcedure, allow: everyone,
		req: func(_ *world) proto.Message { return &campaignsv1.ListMyCampaignsRequest{} },
	},
	{
		procedure: campaignsv1connect.CampaignServiceGetTableRulesProcedure, allow: membersAndPending, why: "a pending member needs the ways to make ability scores to create their character",
		req: func(w *world) proto.Message {
			return &campaignsv1.GetTableRulesRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: campaignsv1connect.CampaignServiceListInvitesProcedure, allow: masterOnlyRead, why: "invite tokens let anyone in",
		req: func(w *world) proto.Message {
			return &campaignsv1.ListInvitesRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: campaignsv1connect.CampaignServiceListPendingMembersProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &campaignsv1.ListPendingMembersRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: campaignsv1connect.CampaignDocumentServiceGetCampaignDocumentProcedure, allow: masterOnlyRead, why: "the document holds prep notes and spoilers",
		req: func(w *world) proto.Message {
			return &campaignsv1.GetCampaignDocumentRequest{CampaignId: w.campaign}
		},
	},

	// ===== InventoryService
	{
		procedure: charactersv1connect.InventoryServiceGetInventoryProcedure, label: "Ana's inventory", allow: onlyAna, why: "an unidentified item shows its owner only its look (RN-10)",
		req: func(w *world) proto.Message {
			return &charactersv1.GetInventoryRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}
		},
	},
	{
		procedure: charactersv1connect.InventoryServiceGetInventoryProcedure, label: "Caio's inventory", allow: onlyCaio,
		req: func(w *world) proto.Message {
			return &charactersv1.GetInventoryRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId()}
		},
	},
	{
		procedure: charactersv1connect.InventoryServiceListItemCatalogProcedure, allow: members, why: "the catalog is the SRD's and names every item; the table's own unidentified ones are in it as the book has them",
		ignore: []string{"item-identity-Ana", "item-identity-Caio"},
		req: func(w *world) proto.Message {
			return &charactersv1.ListItemCatalogRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: charactersv1connect.InventoryServiceListItemLogProcedure, allow: members, why: "a player reads the lines about their own characters, with the look of what is unidentified",
		req: func(w *world) proto.Message {
			return &charactersv1.ListItemLogRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: charactersv1connect.InventoryServiceListItemRestsProcedure, allow: masterOnlyRead, why: "the short rest's list names the items to attune and to identify",
		req: func(w *world) proto.Message {
			return &charactersv1.ListItemRestsRequest{CampaignId: w.campaign}
		},
	},

	// ===== CharacterService
	{
		procedure: charactersv1connect.CharacterServiceListCharactersProcedure, allow: membersAndPending, why: "a pending member reads their own pending character",
		req: func(w *world) proto.Message {
			return &charactersv1.ListCharactersRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceGetCharacterProcedure, label: "Ana's character", allow: onlyAna,
		req: func(w *world) proto.Message {
			return &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceGetCharacterProcedure, label: "Caio's character", allow: onlyCaio,
		req: func(w *world) proto.Message {
			return &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId()}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceGetCharacterProcedure, label: "the boss NPC", allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.boss.GetId()}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceGetCharacterProcedure, label: "an NPC on the stage", allow: masterOnlyRead, why: "a player reads an NPC on the stage through GetOpenScene, never as a character",
		req: func(w *world) proto.Message {
			return &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.merchant.GetId()}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceGetMasterNotesProcedure, label: "Ana's character", allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &charactersv1.GetMasterNotesRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceGetMasterNotesProcedure, label: "Caio's character", allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &charactersv1.GetMasterNotesRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId()}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceGetCharacterProcedure, label: "an NPC in Ana's sight", allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.seenNPC.GetId()}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceGetCharacterProcedure, label: "a hidden NPC", allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.hiddenNPC.GetId()}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceGetCharacterProcedure, label: "an NPC out of the scene", allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.offstage.GetId()}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceGetCharacterProcedure, label: "an NPC made from a creature", allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.bandit.GetId()}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceGetMasterNotesProcedure, label: "the boss NPC", allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &charactersv1.GetMasterNotesRequest{CampaignId: w.campaign, CharacterId: w.boss.GetId()}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceGetAbilityRollsProcedure, allow: membersAndPending, notMaster: true, why: "a player's own stored rolls; the master's NPCs need none",
		req: func(w *world) proto.Message {
			return &charactersv1.GetAbilityRollsRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceGetLevelUpOptionsProcedure, label: "Ana's character", allow: onlyAna,
		req: func(w *world) proto.Message {
			return &charactersv1.GetLevelUpOptionsRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId(), ClassKey: "class:wizard"}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceListLevelUpsProcedure, label: "Ana's character", allow: masterOnlyRead, why: "the level-up history is the master's list",
		req: func(w *world) proto.Message {
			return &charactersv1.ListLevelUpsRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceListCharacterCreaturesProcedure, label: "Ana's character", allow: onlyAna,
		req: func(w *world) proto.Message {
			return &charactersv1.ListCharacterCreaturesRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceListCharacterCreaturesProcedure, label: "Caio's character", allow: onlyCaio,
		req: func(w *world) proto.Message {
			return &charactersv1.ListCharacterCreaturesRequest{CampaignId: w.campaign, CharacterId: w.toren.GetId()}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceGetCharacterProcedure, label: "an NPC put on the stage by the stream test", allow: masterOnlyRead, why: "a player reads a stage NPC through GetOpenScene, never as a character",
		req: func(w *world) proto.Message {
			return &charactersv1.GetCharacterRequest{CampaignId: w.campaign, CharacterId: w.stage2.GetId()}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceListWildShapeFormsProcedure, label: "Ana's character", allow: onlyAna,
		req: func(w *world) proto.Message {
			return &charactersv1.ListWildShapeFormsRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}
		},
	},
	{
		procedure: charactersv1connect.CharacterServiceGetSummonOptionsProcedure, label: "Ana's character", allow: onlyAna,
		req: func(w *world) proto.Message {
			return &charactersv1.GetSummonOptionsRequest{CampaignId: w.campaign, CharacterId: w.pens.GetId()}
		},
	},

	// ===== ContentService (the rules: public SRD data plus the table's own content)
	{
		procedure: rulesv1connect.ContentServiceListContentProcedure, allow: membersAndPending, why: "a pending member needs the catalog to create their character",
		req: func(w *world) proto.Message { return &rulesv1.ListContentRequest{CampaignId: w.campaign} },
	},
	{
		procedure: rulesv1connect.ContentServiceListContentProcedure, label: "for the caller's own character", allow: membersAndPending, why: "a pending member reads the content with their pending character's id, to create it",
		reqFor: func(w *world, p *person) proto.Message {
			return &rulesv1.ListContentRequest{CampaignId: w.campaign, CharacterId: w.ownCharacter(p).GetId()}
		},
	},
	{
		procedure: rulesv1connect.ContentServiceListCreaturesProcedure, allow: members, ignore: []string{"monster-key"}, why: "the bestiary is public SRD data: it names every creature",
		req: func(w *world) proto.Message {
			return &rulesv1.ListCreaturesRequest{CampaignId: w.campaign, PageSize: 400}
		},
	},
	{
		procedure: rulesv1connect.ContentServiceGetCreatureProcedure, allow: members, ignore: []string{"monster-key"}, why: "a stat block of the SRD is public",
		req: func(w *world) proto.Message {
			return &rulesv1.GetCreatureRequest{CampaignId: w.campaign, Key: "monster:bandit"}
		},
	},
	{
		procedure: rulesv1connect.ContentServiceListLightPresetsProcedure, allow: members,
		req: func(w *world) proto.Message {
			return &rulesv1.ListLightPresetsRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: rulesv1connect.ContentServiceListSpellsProcedure, allow: members,
		req: func(w *world) proto.Message { return &rulesv1.ListSpellsRequest{CampaignId: w.campaign} },
	},
	{
		procedure: rulesv1connect.ContentServiceGetSpellDetailsProcedure, label: "an archived table spell", allow: masterOnlyRead, why: "an archived spell is unknown to a player (not_found)",
		req: func(w *world) proto.Message {
			return &rulesv1.GetSpellDetailsRequest{CampaignId: w.campaign, SpellKey: w.keyOf("spell-archived")}
		},
	},
	{
		procedure: rulesv1connect.ContentServiceGetSpellDetailsProcedure, label: "a live table spell", allow: membersAndPending,
		req: func(w *world) proto.Message {
			return &rulesv1.GetSpellDetailsRequest{CampaignId: w.campaign, SpellKey: w.keyOf("spell-live")}
		},
	},
	{
		procedure: rulesv1connect.ContentServiceListTrapPresetsProcedure, allow: members, why: "the SRD's sample traps and their numbers are public; the master's own traps are not here",
		req: func(w *world) proto.Message {
			return &rulesv1.ListTrapPresetsRequest{CampaignId: w.campaign}
		},
	},

	// ===== TableContentService (the master's editor)
	{
		procedure: rulesv1connect.TableContentServiceListTableEntriesProcedure, allow: members, why: "a player reads the entries that are not archived and not off (RN-23)",
		req: func(w *world) proto.Message {
			return &rulesv1.ListTableEntriesRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: rulesv1connect.TableContentServiceListOptionSwitchesProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &rulesv1.ListOptionSwitchesRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: rulesv1connect.TableContentServiceGetEffectMenuProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message { return &rulesv1.GetEffectMenuRequest{CampaignId: w.campaign} },
	},
	{
		procedure: rulesv1connect.TableContentServiceGetClassTableDefaultsProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &rulesv1.GetClassTableDefaultsRequest{CampaignId: w.campaign}
		},
	},

	// ===== PlayService
	{
		procedure: playv1connect.PlayServiceGetLiveSessionProcedure, allow: members,
		req: func(w *world) proto.Message { return &playv1.GetLiveSessionRequest{CampaignId: w.campaign} },
	},
	{
		procedure: playv1connect.PlayServiceListGameSessionsProcedure, allow: members,
		req: func(w *world) proto.Message {
			return &playv1.ListGameSessionsRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: playv1connect.PlayServiceListOpenGameSessionsProcedure, allow: everyone,
		req: func(_ *world) proto.Message { return &playv1.ListOpenGameSessionsRequest{} },
	},
	{
		procedure: playv1connect.PlayServiceGetOpenSceneProcedure, allow: members,
		req: func(w *world) proto.Message { return &playv1.GetOpenSceneRequest{CampaignId: w.campaign} },
	},
	{
		procedure: playv1connect.PlayServiceListLeftImagesProcedure, allow: members,
		req: func(w *world) proto.Message { return &playv1.ListLeftImagesRequest{CampaignId: w.campaign} },
	},
	{
		procedure: playv1connect.PlayServiceListTrapActivityProcedure, allow: members,
		req: func(w *world) proto.Message {
			return &playv1.ListTrapActivityRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: playv1connect.PlayServiceListTrapDamagesProcedure, allow: masterOnlyRead, why: "the damage a trap is waiting to do is the master's to apply",
		req: func(w *world) proto.Message { return &playv1.ListTrapDamagesRequest{CampaignId: w.campaign} },
	},

	// ===== CombatService
	{
		procedure: playv1connect.CombatServiceGetEncounterProcedure, allow: members,
		req: func(w *world) proto.Message { return &playv1.GetEncounterRequest{CampaignId: w.campaign} },
	},
	{
		procedure: playv1connect.CombatServiceListCombatLogProcedure, allow: members,
		req: func(w *world) proto.Message {
			return &playv1.ListCombatLogRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()}
		},
	},
	{
		procedure: playv1connect.CombatServiceGetMoveOptionsProcedure, label: "Ana's combatant", allow: masterOnlyRead, why: "Ana's character is down: a player who is down has no move options, only the master plans her move",
		req: func(w *world) proto.Message {
			return &playv1.GetMoveOptionsRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: w.combatant(w.pens).GetId()}
		},
	},
	{
		procedure: playv1connect.CombatServiceGetMoveOptionsProcedure, label: "the boss", allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &playv1.GetMoveOptionsRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: w.combatant(w.boss).GetId()}
		},
	},
	{
		procedure: playv1connect.CombatServiceGetTurnOptionsProcedure, label: "Ana's combatant", allow: onlyAna, once: true, why: "a player reads their turn options on their own turn",
		req: func(w *world) proto.Message {
			return &playv1.GetTurnOptionsRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: w.combatant(w.pens).GetId()}
		},
	},
	{
		procedure: playv1connect.CombatServiceGetTurnOptionsProcedure, label: "the boss", allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &playv1.GetTurnOptionsRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId(), CombatantId: w.combatant(w.boss).GetId()}
		},
	},

	// ===== PuzzleService
	{
		procedure: playv1connect.PuzzleServiceListShownPuzzlesProcedure, allow: members,
		req: func(w *world) proto.Message {
			return &playv1.ListShownPuzzlesRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: playv1connect.PuzzleServiceGetPuzzleRunProcedure, label: "the lock on screen", allow: members,
		req: func(w *world) proto.Message {
			return &playv1.GetPuzzleRunRequest{CampaignId: w.campaign, PuzzleId: w.puzzles["shown"].GetId()}
		},
	},
	{
		procedure: playv1connect.PuzzleServiceGetPuzzleRunProcedure, label: "the riddle on screen", allow: members,
		req: func(w *world) proto.Message {
			return &playv1.GetPuzzleRunRequest{CampaignId: w.campaign, PuzzleId: w.puzzles["riddle-shown"].GetId()}
		},
	},
	{
		procedure: playv1connect.PuzzleServiceGetPuzzleRunProcedure, label: "the split clue", allow: members,
		req: func(w *world) proto.Message {
			return &playv1.GetPuzzleRunRequest{CampaignId: w.campaign, PuzzleId: w.puzzles["split"].GetId()}
		},
	},
	{
		procedure: playv1connect.PuzzleServiceGetPuzzleRunProcedure, label: "a riddle not shown", allow: masterOnlyRead, notMaster: true, why: "the player's read of a puzzle that is not shown is not_found for the master too (the master reads GetMasterPuzzleRun)",
		req: func(w *world) proto.Message {
			return &playv1.GetPuzzleRunRequest{CampaignId: w.campaign, PuzzleId: w.puzzles["riddle-hidden"].GetId()}
		},
	},
	{
		procedure: playv1connect.PuzzleServiceGetPuzzleRunProcedure, label: "an archived cipher", allow: masterOnlyRead, notMaster: true,
		req: func(w *world) proto.Message {
			return &playv1.GetPuzzleRunRequest{CampaignId: w.campaign, PuzzleId: w.puzzles["cipher"].GetId()}
		},
	},
	{
		procedure: playv1connect.PuzzleServiceGetPuzzleProcedure, label: "the lock on screen", allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &playv1.GetPuzzleRequest{CampaignId: w.campaign, PuzzleId: w.puzzles["shown"].GetId()}
		},
	},
	{
		procedure: playv1connect.PuzzleServiceListPuzzlesProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &playv1.ListPuzzlesRequest{CampaignId: w.campaign, IncludeArchived: true}
		},
	},
	{
		procedure: playv1connect.PuzzleServiceListSessionPuzzlesProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &playv1.ListSessionPuzzlesRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: playv1connect.PuzzleServiceGetMasterPuzzleRunProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &playv1.GetMasterPuzzleRunRequest{CampaignId: w.campaign, PuzzleId: w.puzzles["shown"].GetId()}
		},
	},

	// ===== EncounterService (the encounter planner: all of it is the master's)
	{
		procedure: playv1connect.EncounterServiceGetBattleEncounterProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &playv1.GetBattleEncounterRequest{CampaignId: w.campaign, MapPointId: w.pts["battle"].GetId()}
		},
	},
	{
		procedure: playv1connect.EncounterServiceListBattleEncountersProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &playv1.ListBattleEncountersRequest{CampaignId: w.campaign, MapId: w.fogMap}
		},
	},
	{
		procedure: playv1connect.EncounterServiceEvaluateEncounterProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &playv1.EvaluateEncounterRequest{CampaignId: w.campaign, Entries: []*playv1.MonsterGroup{{CreatureKey: "monster:ogre", Count: 1}}}
		},
	},
	{
		procedure: playv1connect.EncounterServiceGenerateEncounterProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &playv1.GenerateEncounterRequest{CampaignId: w.campaign, Band: playv1.EncounterBand_ENCOUNTER_BAND_MODERATE, Seed: 7}
		},
	},
	{
		procedure: playv1connect.EncounterServiceListEncounterSwapsProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &playv1.ListEncounterSwapsRequest{CampaignId: w.campaign, CreatureKey: "monster:ogre"}
		},
	},

	// ===== MapService
	{
		procedure: mapsv1connect.MapServiceListMapsProcedure, allow: members,
		req: func(w *world) proto.Message { return &mapsv1.ListMapsRequest{CampaignId: w.campaign} },
	},
	{
		procedure: mapsv1connect.MapServiceGetMapProcedure, label: "the cave, with the fog", allow: members,
		req: func(w *world) proto.Message {
			return &mapsv1.GetMapRequest{CampaignId: w.campaign, MapId: w.fogMap}
		},
	},
	{
		procedure: mapsv1connect.MapServiceGetMapProcedure, label: "the cave, as a character", allow: masterOnlyRead,
		why: "as_character_id is the master's way to see what a player sees: a player asking as someone else's character is refused",
		req: func(w *world) proto.Message {
			return &mapsv1.GetMapRequest{CampaignId: w.campaign, MapId: w.fogMap, AsCharacterId: w.pens.GetId()}
		},
	},
	{
		procedure: mapsv1connect.MapServiceGetMapProcedure, label: "a hidden map", allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &mapsv1.GetMapRequest{CampaignId: w.campaign, MapId: w.hiddenMap}
		},
	},
	{
		procedure: mapsv1connect.MapServiceGetMapProcedure, label: "a generated dungeon, hidden", allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &mapsv1.GetMapRequest{CampaignId: w.campaign, MapId: w.dungeonMap}
		},
	},
	{
		procedure: mapsv1connect.MapServiceGetMapLayersProcedure, label: "the cave", allow: members,
		req: func(w *world) proto.Message {
			return &mapsv1.GetMapLayersRequest{CampaignId: w.campaign, MapId: w.fogMap}
		},
	},
	{
		procedure: mapsv1connect.MapServiceGetMapLayersProcedure, label: "a hidden map", allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &mapsv1.GetMapLayersRequest{CampaignId: w.campaign, MapId: w.dungeonMap}
		},
	},
	{
		procedure: mapsv1connect.MapServiceGetMapVisionProcedure, label: "the cave", allow: members,
		req: func(w *world) proto.Message {
			return &mapsv1.GetMapVisionRequest{CampaignId: w.campaign, MapId: w.fogMap}
		},
	},
	{
		procedure: mapsv1connect.MapServiceGetMapVisionProcedure, label: "a hidden map", allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &mapsv1.GetMapVisionRequest{CampaignId: w.campaign, MapId: w.dungeonMap}
		},
	},
	{
		procedure: mapsv1connect.MapServiceGetTrapNoticersProcedure, allow: masterOnlyRead, why: "who would notice a trap: passive Perception of every character",
		req: func(w *world) proto.Message {
			return &mapsv1.GetTrapNoticersRequest{CampaignId: w.campaign, MapId: w.fogMap, PointId: w.pts["trap-hidden"].GetId()}
		},
	},

	// ===== GalleryService, ImageGenerationService, DungeonService, TreasureService (the master's tools)
	{
		procedure: mapsv1connect.GalleryServiceListGalleryImagesProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &mapsv1.ListGalleryImagesRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: mapsv1connect.ImageGenerationServiceGetImageGenerationStatusProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &mapsv1.GetImageGenerationStatusRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: mapsv1connect.ImageGenerationServiceGetImageGenerationProcedure, allow: masterOnlyRead, why: "the request holds the master's prompt and the picture not yet shown",
		req: func(w *world) proto.Message {
			return &mapsv1.GetImageGenerationRequest{CampaignId: w.campaign, GenerationId: w.generation}
		},
	},
	{
		procedure: mapsv1connect.ImageGenerationServiceListImageEditsProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &mapsv1.ListImageEditsRequest{CampaignId: w.campaign, ImageId: w.imgGenerated}
		},
	},
	{
		procedure: mapsv1connect.ImageGenerationServiceGetMapImageReferenceProcedure, allow: masterOnlyRead, why: "the drawing sent to the model names the NPCs the players do not see",
		req: func(w *world) proto.Message {
			return &mapsv1.GetMapImageReferenceRequest{CampaignId: w.campaign, MapId: w.fogMap, Kind: mapsv1.ImageGenerationKind_IMAGE_GENERATION_KIND_MAP_SCENE}
		},
	},
	{
		procedure: mapsv1connect.DungeonServiceGetDungeonRoomsProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &mapsv1.GetDungeonRoomsRequest{CampaignId: w.campaign, MapId: w.dungeonMap}
		},
	},
	{
		procedure: mapsv1connect.DungeonServicePreviewDungeonProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message { return &mapsv1.PreviewDungeonRequest{CampaignId: w.campaign} },
	},
	{
		procedure: mapsv1connect.TreasureServiceGetTreasurePartyProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &mapsv1.GetTreasurePartyRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: mapsv1connect.TreasureServiceGenerateTreasureProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &mapsv1.GenerateTreasureRequest{CampaignId: w.campaign, Mode: mapsv1.TreasureMode_TREASURE_MODE_HOARD, PartyLevel: proto.Int32(5)}
		},
	},
	{
		procedure: mapsv1connect.TreasureServiceGetMagicItemProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &mapsv1.GetMagicItemRequest{CampaignId: w.campaign, Key: "item:ring-of-protection"}
		},
	},

	// ===== NotesService (each player's own)
	{
		procedure: notesv1connect.NotesServiceListNotesProcedure, allow: members, notMaster: true, why: "the notes are the players' own: the master has none",
		req: func(w *world) proto.Message { return &notesv1.ListNotesRequest{CampaignId: w.campaign} },
	},
	{
		procedure: notesv1connect.NotesServiceListNoteScenesProcedure, allow: members, notMaster: true,
		ignore: []string{"fog-point-name", "entrance-name", "fog-point-description"},
		why:    "BY DESIGN (MR-030, confirmed 07/10/2026): a scene the master revealed on a map the players can open is \"discovered\" for the whole group, even where the fog hides the spot from every character; revealing is the master's choice, and the fog only hides the map",
		req:    func(w *world) proto.Message { return &notesv1.ListNoteScenesRequest{CampaignId: w.campaign} },
	},

	// ===== ProgressionService
	{
		procedure: progressionv1connect.ProgressionServiceGetCampaignExperienceProcedure, allow: members,
		req: func(w *world) proto.Message {
			return &progressionv1.GetCampaignExperienceRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: progressionv1connect.ProgressionServiceListMilestonesProcedure, allow: members,
		req: func(w *world) proto.Message {
			return &progressionv1.ListMilestonesRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: progressionv1connect.ProgressionServiceListXPAwardsProcedure, allow: members,
		req: func(w *world) proto.Message {
			return &progressionv1.ListXPAwardsRequest{CampaignId: w.campaign}
		},
	},
	{
		procedure: progressionv1connect.ProgressionServiceListTreasuresToConvertProcedure, allow: masterOnlyRead,
		req: func(w *world) proto.Message {
			return &progressionv1.ListTreasuresToConvertRequest{CampaignId: w.campaign}
		},
	},
}

// readsAfterTheCombat are the reads that exist only when the combat has ended, while the session
// is still open: the highlights. They are asked right after the master ends the combat.
var readsAfterTheCombat = []read{
	{
		procedure: playv1connect.CombatServiceGetCombatHighlightsProcedure, allow: members,
		req: func(w *world) proto.Message {
			return &playv1.GetCombatHighlightsRequest{CampaignId: w.campaign, EncounterId: w.encounter.GetId()}
		},
	},
}

// readsAfterTheSession are the reads that exist only when the session and the combat
// have ended: the master's summary and the highlights. They are asked after the master
// ends them (the last step of TestLeakMatrix), with the same rules as `reads`.
var readsAfterTheSession = []read{
	{
		procedure: playv1connect.PlayServiceGetSessionSummaryProcedure, allow: members,
		req: func(w *world) proto.Message {
			return &playv1.GetSessionSummaryRequest{CampaignId: w.campaign, GameSessionId: w.session}
		},
	},
}
