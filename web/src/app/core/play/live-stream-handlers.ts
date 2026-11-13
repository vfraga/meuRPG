import type { FreeRollVm } from '../free-dice/free-roll';
import type { SessionMessage } from '../../../gen/meurpg/play/v1/session_messages_pb';
import type { CombatantMove, TurnChange } from '../combat/combat-state';
import type { AttackHit } from '../combat/own-hit';
import type { ShownImageVm, TableScreenChange, VitalsVm } from './live-session.types';

/** What the page hands the live stream: one handler for each kind of message it cares about. */
export interface LiveStreamHandlers {
  /** The server subscribed us: read the snapshot now (and after every
   * reconnection, so a missed event never leaves the screen stale). */
  onReady(): void;
  onVitals(vitals: VitalsVm): void;
  /** `current_map_changed`: another map (or none) on the table. */
  onCurrentMap?(mapId: string | null): void;
  /** `map_changed`: something on this map changed, read it again. */
  onMapChanged?(mapId: string): void;
  /** `token_moved`: a token has a new position, with no reading. */
  onTokenMoved?(move: { mapId: string; characterId: string; xBp: number; yBp: number }): void;
  /** `vision_changed`: what the player sees of a fog map changed, read it again. */
  onVisionChanged?(mapId: string): void;
  /** `shown_image_changed`: an image on the players' screens, or none. */
  onShownImage?(image: ShownImageVm | null): void;
  /** `left_images_changed`: read the list of left images again. */
  onLeftImages?(): void;
  /** `encounter_changed`: the combat changed; the page reads it again when
   * `revision` is newer than its copy. */
  onEncounterChanged?(change: { encounterId: string; revision: number; mode?: number }): void;
  /** `reaction_window_opened`: a window waits for this player (or the master); the page reads the combat again. */
  onReactionWindowOpened?(opened: { encounterId: string; windowId: string }): void;
  /** `reaction_window_closed`: a window is no longer open; `text` says why when it closed by itself. */
  onReactionWindowClosed?(closed: {
    encounterId: string;
    windowId: string;
    closedByItself: boolean;
    text: string;
  }): void;
  /** `turn_changed`: applied in place, per audience. */
  onTurnChanged?(turn: TurnChange): void;
  /** `combatant_moved`: applied in place. */
  onCombatantMoved?(move: CombatantMove): void;
  /** `combat_log_changed`: the log has a new entry, read it again. `full` (the master only): the log was rewritten, read all of it. */
  onCombatLogChanged?(full: boolean): void;
  /** A heartbeat said the session's hint number is ahead of what this stream saw: a hint was lost. Read everything a `ready` reads, once.
   * Resolves `true` when the read worked; on `false` the stream keeps its number and the next heartbeat asks again. */
  onBehind?(): Promise<boolean>;
  /** `xp_changed`: the campaign's XP changed, read it again. */
  onXpChanged?(): void;
  /** `scene_changed` and `scene_check_rolled`: read the open scene again. */
  onSceneChanged?(): void;
  /** `notes_changed`: the master revealed a clue to this player, read the notes again. */
  onNotesChanged?(): void;
  /** `stage_changed` (MR-031): the stage lives in the open scene, so the page
   * reads the scene again; the event names nobody. */
  onStageChanged?(): void;
  /** `trap_noticed` (MR-035): this player's character noticed a trap; only they get it. */
  onTrapNoticed?(notice: { mapId: string; pointId: string }): void;
  /** `creatures_changed` (MR-037): a character's creatures changed outside a combat, read them again. */
  onCreaturesChanged?(): void;
  /** `spell_casts_changed` (MR-048): a cast outside a combat changed; the casts panel reads them again. */
  onSpellCastsChanged?(): void;
  /** `group_check_changed` (W7-X): a group check was asked, answered or closed; the page reads it again. */
  onGroupCheckChanged?(): void;
  /** `content_changed` (10.1d): the table's content changed; a screen that shows the catalog reads it again. */
  onContentChanged?(): void;
  /** `character_changes_requested`, `character_resubmitted` or `character_revived`: that character changed; read it again. */
  onCharacterChanged?(characterId: string): void;
  /** `character_revived`: a dead character lives again (`onCharacterChanged` is told too); the vitals change with it. */
  onCharacterRevived?(characterId: string): void;
  /** `revivify_changed`: a Revivify cast outside a combat was asked or answered; the master reads the casts again. */
  onRevivifyChanged?(): void;
  /** `puzzle_changed` (MR-038): a puzzle changed; the page reads it again (the server already throttles the hint). */
  onPuzzleChanged?(puzzleId: string): void;
  /** `session_message_added` (MR-051): a line of the conversation, to be kept once by id. */
  onSessionMessage?(message: SessionMessage): void;
  /** `master_typing` (MR-051): show "O mestre está escrevendo…" for this many seconds; it carries no text. */
  onMasterTyping?(expiresInSeconds: number): void;
  /** `table_screen_changed` (master only): the TV at the table connected, left or got a new choice. */
  onTableScreenChanged?(change: TableScreenChange): void;
  /** The master applied the damage of an attack to this player's character. */
  onAttackHit?(hit: AttackHit): void;
  /** `held_rolls_changed`: the questions of the held rolls changed (content-free); read the list again. */
  onHeldRollsChanged?(): void;
  /** `free_roll_rolled` (MR-052): someone rolled at the table (never a secret roll); the log gets it and the dice tumble. */
  onFreeRollRolled?(roll: FreeRollVm): void;
  /** `calling_you`: the master is calling this player; the page alerts (vibration, chime, shake) and says it. */
  onCallingYou?(): void;
  /** `session_ended`, or `NO_OPEN_SESSION` when (re)connecting. */
  onEnded(): void;
  /** `not_found` or `unauthenticated`: no reconnecting. */
  onFatal(kind: 'no-access' | 'signed-out'): void;
}
