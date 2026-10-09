package leaktest

import (
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/campaigns/v1/campaignsv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/characters/v1/charactersv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/identity/v1/identityv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/maps/v1/mapsv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/notes/v1/notesv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1/playv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/progression/v1/progressionv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/rules/v1/rulesv1connect"
	"github.com/PuraFome/meuRPG/backend/gen/meurpg/system/v1/systemv1connect"
)

// Every procedure of the API is in exactly one of three places (TestEveryProcedureIsClassified
// fails otherwise, so a new RPC must be classified to pass):
//
//   - the table of reads (reads_test.go): every call that returns data, asked of every person;
//
//   - the table of reads after the session (readsAfterTheSession, below);
//
//   - this table of the rest, each with a class and the reason:
//
//     masterWrite       only the master may call it. A player, a pending member and a stranger
//     are refused with whatever the request holds (TestMasterWritesAreRefused),
//     and no refusal says what the request aimed at.
//     playerAction      a member may call it for their own character, notes or turn. The
//     scripted actions (actions_test.go) and the probe (TestPlayerActionsAnswerNothingHidden)
//     aim it at what is hidden and check the answers.
//     notACampaignCall  nothing of a campaign: the caller's own account, the server's version.
//     streamed          the live stream: stream_test.go.
//
// To classify a new RPC, add one line. If it is a read, add a row to `reads` instead.
type class int

const (
	masterWrite class = iota + 1
	playerAction
	notACampaignCall
	streamed
)

type classified struct {
	class class
	why   string
}

// masterOnlyWhy is the reason of every masterWrite: the .proto says "Only the master may call it".
const masterOnlyWhy = "only the master may call it (see the .proto comment)"

var notReads = map[string]classified{
	campaignsv1connect.CampaignDocumentServiceUpdateCampaignDocumentProcedure: {masterWrite, masterOnlyWhy},
	campaignsv1connect.CampaignServiceAcceptInviteProcedure:                   {playerAction, "anyone with an invite token may accept it; the answer is the campaign's id and name"},
	campaignsv1connect.CampaignServiceCreateCampaignProcedure:                 {playerAction, "anyone may start a campaign of their own"},
	campaignsv1connect.CampaignServiceCreateInviteProcedure:                   {masterWrite, masterOnlyWhy},
	campaignsv1connect.CampaignServiceRemovePendingMemberProcedure:            {masterWrite, masterOnlyWhy},
	campaignsv1connect.CampaignServiceRevokeInviteProcedure:                   {masterWrite, masterOnlyWhy},
	campaignsv1connect.CampaignServiceSetCampaignDiceModeProcedure:            {masterWrite, masterOnlyWhy},
	campaignsv1connect.CampaignServiceSetCampaignXpModeProcedure:              {masterWrite, masterOnlyWhy},
	campaignsv1connect.CampaignServiceSetMyDicePreferenceProcedure:            {playerAction, "a member sets their own preference"},
	campaignsv1connect.CampaignServiceSetTableRulesProcedure:                  {masterWrite, masterOnlyWhy},
	charactersv1connect.CharacterServiceAdjustCreatureHitPointsProcedure:      {masterWrite, masterOnlyWhy},
	charactersv1connect.CharacterServiceApproveCharacterProcedure:             {masterWrite, masterOnlyWhy},
	charactersv1connect.CharacterServiceCreateCharacterProcedure:              {playerAction, "a player (or a pending member) makes their own character"},
	charactersv1connect.CharacterServiceCreateNpcFromCreatureProcedure:        {masterWrite, masterOnlyWhy},
	charactersv1connect.CharacterServiceDismissCreatureProcedure:              {playerAction, "a player dismisses their own creature"},
	charactersv1connect.CharacterServiceGiveCreatureProcedure:                 {masterWrite, masterOnlyWhy},
	charactersv1connect.CharacterServiceLevelUpCharacterProcedure:             {playerAction, "a player levels their own character"},
	charactersv1connect.CharacterServiceMarkCharacterDeadProcedure:            {masterWrite, masterOnlyWhy},
	charactersv1connect.CharacterServicePreviewCharacterProcedure:             {playerAction, "a player previews the sheet they are making or editing; the checks are the save's"},
	charactersv1connect.CharacterServicePreviewLevelUpProcedure:               {playerAction, "a player previews their own level-up"},
	charactersv1connect.CharacterServiceRejectCharacterProcedure:              {masterWrite, masterOnlyWhy},
	charactersv1connect.CharacterServiceRenameCreatureProcedure:               {playerAction, "a player renames their own creature"},
	charactersv1connect.CharacterServiceRollAbilityScoresProcedure:            {playerAction, "a player rolls the scores of their next character"},
	charactersv1connect.CharacterServiceRollLevelUpHitPointsProcedure:         {playerAction, "a player rolls their own level-up hit die"},
	charactersv1connect.CharacterServiceSetStoryEditingProcedure:              {masterWrite, masterOnlyWhy},
	charactersv1connect.CharacterServiceUpdateCharacterProcedure:              {playerAction, "a player edits their own sheet"},
	charactersv1connect.CharacterServiceUpdateCharacterStoryProcedure:         {playerAction, "a player edits their own story"},
	charactersv1connect.CharacterServiceUpdateMasterNotesProcedure:            {masterWrite, masterOnlyWhy},
	charactersv1connect.InventoryServiceGiveItemsProcedure:                    {playerAction, "a player gives their own character equipment and free text; the master anything"},
	charactersv1connect.InventoryServiceSetEquippedProcedure:                  {playerAction, "a player wears and wields their own things"},
	charactersv1connect.InventoryServiceRemoveItemProcedure:                   {playerAction, "a player removes their own mundane items"},
	charactersv1connect.InventoryServiceTransferItemProcedure:                 {playerAction, "a player hands their own item to another player's character"},
	charactersv1connect.InventoryServiceAdjustCoinsProcedure:                  {playerAction, "a player counts their own coins"},
	charactersv1connect.InventoryServiceSetCoinsProcedure:                     {playerAction, "a player sets their own coins"},
	charactersv1connect.InventoryServiceRequestItemRestProcedure:              {playerAction, "a player marks their own item for the master's short rest"},
	charactersv1connect.InventoryServiceResolveItemRestProcedure:              {masterWrite, masterOnlyWhy},
	charactersv1connect.InventoryServiceConfirmItemRestsProcedure:             {masterWrite, masterOnlyWhy},
	charactersv1connect.InventoryServiceUseItemProcedure:                      {playerAction, "a player drinks their own potion outside combat"},
	charactersv1connect.InventoryServiceUseScrollProcedure:                    {playerAction, "a player reads their own scroll"},
	charactersv1connect.InventoryServiceSpendChargesProcedure:                 {playerAction, "a player spends the charges of their own item"},
	charactersv1connect.InventoryServiceRecoverAmmunitionProcedure:            {playerAction, "a player picks up their own spent ammunition"},
	charactersv1connect.InventoryServicePreviewTextToItemsProcedure:           {playerAction, "a player previews turning their own text into items"},
	charactersv1connect.InventoryServiceConfirmTextToItemsProcedure:           {playerAction, "a player turns their own text into items"},
	charactersv1connect.InventoryServiceRegainChargesProcedure:                {masterWrite, masterOnlyWhy},
	charactersv1connect.InventoryServiceUpdateItemProcedure:                   {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceUseItemProcedure:                               {playerAction, "a player uses an item with their own combatant"},
	identityv1connect.IdentityServiceGetMeProcedure:                           {notACampaignCall, "the caller's own account: nothing of a campaign"},
	identityv1connect.IdentityServiceSignOutProcedure:                         {notACampaignCall, "the caller's own session"},
	identityv1connect.IdentityServiceUpdateProfileProcedure:                   {notACampaignCall, "the caller's own profile"},
	identityv1connect.IdentityServiceCountOtherSessionsProcedure:              {notACampaignCall, "a count of the caller's own other sessions: nothing of a campaign"},
	identityv1connect.IdentityServiceSignOutOtherSessionsProcedure:            {notACampaignCall, "ends the caller's own other sessions"},
	mapsv1connect.DungeonServiceCreateDungeonMapProcedure:                     {masterWrite, masterOnlyWhy},
	mapsv1connect.DungeonServicePlaceDungeonSceneProcedure:                    {masterWrite, masterOnlyWhy},
	mapsv1connect.DungeonServiceRedrawDungeonMapProcedure:                     {masterWrite, masterOnlyWhy},
	mapsv1connect.GalleryServiceDeleteGalleryImageProcedure:                   {masterWrite, masterOnlyWhy},
	mapsv1connect.GalleryServiceRenameGalleryImageProcedure:                   {masterWrite, masterOnlyWhy},
	mapsv1connect.ImageGenerationServiceCancelImageGenerationProcedure:        {masterWrite, masterOnlyWhy},
	mapsv1connect.ImageGenerationServiceEditGeneratedImageProcedure:           {masterWrite, masterOnlyWhy},
	mapsv1connect.ImageGenerationServiceGenerateMapImageProcedure:             {masterWrite, masterOnlyWhy},
	mapsv1connect.ImageGenerationServiceGenerateSceneImageProcedure:           {masterWrite, masterOnlyWhy},
	mapsv1connect.ImageGenerationServiceUseGeneratedImageAsMapImageProcedure:  {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceAddSceneActionProcedure:                           {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceAddSceneClueProcedure:                             {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceCreateMapProcedure:                                {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceCreateMapPointProcedure:                           {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceDeleteMapProcedure:                                {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceDeleteMapPointProcedure:                           {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceDisarmTrapProcedure:                               {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceForgetMapVisionProcedure:                          {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceMarkTreasureFoundProcedure:                        {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceMoveSceneActionProcedure:                          {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceMoveSceneClueProcedure:                            {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServicePaintMapCellsProcedure:                            {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServicePlaceMapTokenProcedure:                            {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceRemoveMapTokenProcedure:                           {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceRemoveSceneActionProcedure:                        {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceRemoveSceneClueProcedure:                          {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceRevealSceneClueProcedure:                          {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceRevealTrapProcedure:                               {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceSetMapFogProcedure:                                {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceSetMapGridProcedure:                               {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceSetMapPointRevealedProcedure:                      {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceSetMapRevealedProcedure:                           {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceSetMapTokenHiddenProcedure:                        {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceUnmarkTreasureFoundProcedure:                      {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceUpdateMapProcedure:                                {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceUpdateMapPointProcedure:                           {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceUpdateSceneActionProcedure:                        {masterWrite, masterOnlyWhy},
	mapsv1connect.MapServiceUpdateSceneClueProcedure:                          {masterWrite, masterOnlyWhy},
	mapsv1connect.TreasureServicePlaceTreasureProcedure:                       {masterWrite, masterOnlyWhy},
	notesv1connect.NotesServiceCreateNoteProcedure:                            {playerAction, "a player writes their own note"},
	notesv1connect.NotesServiceDeleteNoteProcedure:                            {playerAction, "a player deletes their own note"},
	notesv1connect.NotesServiceUpdateNoteProcedure:                            {playerAction, "a player edits their own note"},
	playv1connect.CombatServiceAddCombatantsProcedure:                         {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceAddMonstersProcedure:                           {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceAdjustCombatantHitPointsProcedure:              {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceApplyPendingDamageProcedure:                    {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceBeginCombatProcedure:                           {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceCastSpellProcedure:                             {playerAction, "a player casts with their own combatant"},
	playv1connect.CombatServiceConfirmDeathProcedure:                          {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceDeclineOpportunityProcedure:                    {playerAction, "a player declines an opportunity attack of their own combatant"},
	playv1connect.CombatServiceDeclineReactionProcedure:                       {playerAction, "a player declines a reaction of their own combatant"},
	playv1connect.CombatServiceDiscardPendingDamageProcedure:                  {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceEndConcentrationProcedure:                      {playerAction, "a player ends their own concentration"},
	playv1connect.CombatServiceEndEncounterProcedure:                          {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceEndTurnProcedure:                               {playerAction, "a player ends their own turn"},
	playv1connect.CombatServiceMoveCombatantProcedure:                         {playerAction, "a player moves their own combatant"},
	playv1connect.CombatServiceOfferOpportunityProcedure:                      {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceRemoveCombatantProcedure:                       {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceRollAttackProcedure:                            {playerAction, "a player attacks with their own combatant"},
	playv1connect.CombatServiceRollDamageProcedure:                            {playerAction, "a player rolls the damage of their own attack"},
	playv1connect.CombatServiceRollDeathSaveProcedure:                         {playerAction, "a player rolls the death save of their own character"},
	playv1connect.CombatServiceSetCombatantConditionsProcedure:                {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceSetCombatantCoverProcedure:                     {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceSetCombatantHiddenProcedure:                    {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceSetCombatantSideProcedure:                      {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceSetInitiativeOrderProcedure:                    {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceSkipOpportunityProcedure:                       {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceSpendMovementProcedure:                         {playerAction, "a player spends their own movement"},
	playv1connect.CombatServiceStartEncounterProcedure:                        {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceSubmitInitiativeProcedure:                      {playerAction, "a player rolls their own initiative"},
	playv1connect.CombatServiceTakeActionProcedure:                            {playerAction, "a player takes an action with their own combatant"},
	playv1connect.CombatServiceUndoLastActionProcedure:                        {masterWrite, masterOnlyWhy},
	playv1connect.CombatServiceUseReactionProcedure:                           {playerAction, "a player answers a reaction prompt of their own combatant"},
	playv1connect.CombatServiceWithdrawOpportunityProcedure:                   {masterWrite, masterOnlyWhy},
	playv1connect.EncounterServiceClearBattleEncounterProcedure:               {masterWrite, masterOnlyWhy},
	playv1connect.EncounterServiceSaveBattleEncounterProcedure:                {masterWrite, masterOnlyWhy},
	playv1connect.PlayServiceAdjustCharacterVitalsProcedure:                   {masterWrite, masterOnlyWhy},
	playv1connect.PlayServiceApplyTrapDamageProcedure:                         {masterWrite, masterOnlyWhy},
	playv1connect.PlayServiceAssumeWildShapeProcedure:                         {playerAction, "a player turns their own druid into a beast"},
	playv1connect.PlayServiceCastSummonProcedure:                              {playerAction, "a player summons for their own character"},
	playv1connect.PlayServiceCloseSceneProcedure:                              {masterWrite, masterOnlyWhy},
	playv1connect.PlayServiceDiscardTrapDamageProcedure:                       {masterWrite, masterOnlyWhy},
	playv1connect.PlayServiceEndGameSessionProcedure:                          {masterWrite, masterOnlyWhy},
	playv1connect.PlayServiceFireTrapProcedure:                                {masterWrite, masterOnlyWhy},
	playv1connect.PlayServiceGrantSceneAttemptProcedure:                       {masterWrite, masterOnlyWhy},
	playv1connect.PlayServiceLeaveWildShapeProcedure:                          {playerAction, "a player leaves their own Wild Shape"},
	playv1connect.PlayServiceOpenSceneProcedure:                               {masterWrite, masterOnlyWhy},
	playv1connect.PlayServicePutOnStageProcedure:                              {masterWrite, masterOnlyWhy},
	playv1connect.PlayServiceSetCurrentMapProcedure:                           {masterWrite, masterOnlyWhy},
	playv1connect.PlayServiceSetShownImageProcedure:                           {masterWrite, masterOnlyWhy},
	playv1connect.PlayServiceSetSpeakerProcedure:                              {masterWrite, masterOnlyWhy},
	playv1connect.PlayServiceStartFamiliarSightProcedure:                      {playerAction, "a player looks through their own familiar"},
	playv1connect.PlayServiceStartGameSessionProcedure:                        {masterWrite, masterOnlyWhy},
	playv1connect.PlayServiceStopFamiliarSightProcedure:                       {playerAction, "a player stops looking through their own familiar"},
	playv1connect.PlayServiceTakeBackLeftImageProcedure:                       {masterWrite, masterOnlyWhy},
	playv1connect.PlayServiceTakeOffStageProcedure:                            {masterWrite, masterOnlyWhy},
	playv1connect.PlayServiceWatchGameSessionProcedure:                        {streamed, "the stream test (stream_test.go)"},
	playv1connect.PuzzleServiceArchivePuzzleProcedure:                         {masterWrite, masterOnlyWhy},
	playv1connect.PuzzleServiceClosePuzzleProcedure:                           {masterWrite, masterOnlyWhy},
	playv1connect.PuzzleServiceCreatePuzzleProcedure:                          {masterWrite, masterOnlyWhy},
	playv1connect.PuzzleServicePlayPuzzleSequenceProcedure:                    {masterWrite, masterOnlyWhy},
	playv1connect.PuzzleServicePreviewPuzzleCipherProcedure:                   {masterWrite, masterOnlyWhy},
	playv1connect.PuzzleServicePreviewPuzzleStartProcedure:                    {masterWrite, masterOnlyWhy},
	playv1connect.PuzzleServiceReleaseNextPuzzleHintProcedure:                 {masterWrite, masterOnlyWhy},
	playv1connect.PuzzleServiceReseedPuzzleProcedure:                          {masterWrite, masterOnlyWhy},
	playv1connect.PuzzleServiceResetPuzzleProcedure:                           {masterWrite, masterOnlyWhy},
	playv1connect.PuzzleServiceShowPuzzleProcedure:                            {masterWrite, masterOnlyWhy},
	playv1connect.PuzzleServiceUnarchivePuzzleProcedure:                       {masterWrite, masterOnlyWhy},
	playv1connect.PuzzleServiceUpdatePuzzleProcedure:                          {masterWrite, masterOnlyWhy},
	progressionv1connect.ProgressionServiceAddMilestoneProcedure:              {masterWrite, masterOnlyWhy},
	progressionv1connect.ProgressionServiceAwardXPProcedure:                   {masterWrite, masterOnlyWhy},
	progressionv1connect.ProgressionServiceGiveMilestoneToProcedure:           {masterWrite, masterOnlyWhy},
	progressionv1connect.ProgressionServiceMarkMilestoneProcedure:             {masterWrite, masterOnlyWhy},
	progressionv1connect.ProgressionServiceMarkMilestoneReachedProcedure:      {masterWrite, masterOnlyWhy},
	progressionv1connect.ProgressionServiceMoveMilestoneProcedure:             {masterWrite, masterOnlyWhy},
	progressionv1connect.ProgressionServiceRemoveMilestoneProcedure:           {masterWrite, masterOnlyWhy},
	progressionv1connect.ProgressionServiceUndoLastXPAwardProcedure:           {masterWrite, masterOnlyWhy},
	progressionv1connect.ProgressionServiceUpdateMilestoneProcedure:           {masterWrite, masterOnlyWhy},
	rulesv1connect.TableContentServiceArchiveTableEntryProcedure:              {masterWrite, masterOnlyWhy},
	rulesv1connect.TableContentServiceCreateTableEntryProcedure:               {masterWrite, masterOnlyWhy},
	rulesv1connect.TableContentServiceSetOptionSwitchesProcedure:              {masterWrite, masterOnlyWhy},
	rulesv1connect.TableContentServiceUnarchiveTableEntryProcedure:            {masterWrite, masterOnlyWhy},
	rulesv1connect.TableContentServiceUpdateTableEntryProcedure:               {masterWrite, masterOnlyWhy},
	systemv1connect.SystemServiceGetServerInfoProcedure:                       {notACampaignCall, "the server's version: no campaign data"},
}
