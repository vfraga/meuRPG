import { EncounterMode } from '../../../gen/meurpg/play/v1/combat_pb';
import { ownHitOf } from '../../core/combat/own-hit';
import { LiveStream, type LiveStreamHandlers } from '../../core/play/live-stream';
import type { ShownImageVm } from '../../core/play/live-session.types';
import { applyVitals } from '../../core/play/vitals';
import type { LiveSessionLoader } from './live-session-loader';
import type { LiveSessionParts, LiveSessionServices } from './live-session-deps';
import type { LiveSessionState } from './live-session-state';
import type { LiveSessionView } from './live-session-view';

/** How long "O mestre está chamando você." stays on the page if the player does not dismiss it, in ms. */
export const CALLING_NOTICE_MS = 10_000;

/** The stream of the session page: opens it, and does what each of its news asks of the page. */
export class LiveSessionEvents {
  /** Counts the stream's news of the current map and of the shown image: a snapshot that began before one of them
   * does not overwrite it. */
  private mapEvents = 0;
  private imageEvents = 0;
  private callingTimer: ReturnType<typeof setTimeout> | undefined;

  constructor(
    private readonly services: LiveSessionServices,
    private readonly state: LiveSessionState,
    private readonly parts: LiveSessionParts,
    private readonly view: LiveSessionView,
    private readonly loader: LiveSessionLoader,
  ) {}

  /** Clears the timer of the "chamando você" line. */
  dispose(): void {
    clearTimeout(this.callingTimer);
    this.parts.conversation.dispose();
  }

  open(campaignId: string, generation: number): void {
    const stream = new LiveStream({
      open: (signal) => this.services.source.watch(campaignId, signal),
      classify: (err) => this.services.source.classifyError(err),
      document: this.services.document,
      handlers: {
        ...this.sessionHandlers(campaignId, generation),
        ...this.mapHandlers(generation),
        ...this.combatHandlers(generation),
        ...this.tableHandlers(),
      },
    });
    this.state.stream.set(stream);
    stream.start();
  }

  /** The stream's own life and what the player sees of the character: ready, vitals, the casts, the end. */
  private sessionHandlers(campaignId: string, generation: number) {
    const { services, state, parts, loader } = this;
    // What a `ready` reads, and what a heartbeat ahead of the stream's number reads again (a lost hint): the same path.
    const readAll = (): Promise<boolean> => {
      // A missed `xp_changed` while reconnecting leaves nothing stale.
      services.xpChanges.bump();
      // A `content_changed` missed while reconnecting leaves the spells the page read before out of date.
      services.spellCatalog.forget(campaignId);
      void parts.trapBoard.refresh();
      // The first `ready` reads the conversation; a reconnection reads the lines that came meanwhile.
      void parts.conversation.sync();
      return loader.readSnapshot(campaignId, generation, () => ({
        map: this.mapEvents,
        image: this.imageEvents,
      }));
    };
    return {
      onReady: () => void readAll(),
      onBehind: readAll,
      onContentChanged: () => {
        services.spellCatalog.forget(campaignId);
        // The sheet the trap skills come from may have changed with the table's content.
        loader.loadPlayerSheet(campaignId, generation);
      },
      onVitals: (v) => {
        // Looking through a familiar's eyes, or coming back, changes what the player sees.
        const before =
          state.vitals().find((x) => x.characterId === v.characterId)?.familiarSight?.creatureId ??
          '';
        state.vitals.update((list) => applyVitals(list, v));
        if ((v.familiarSight?.creatureId ?? '') !== before) {
          this.loader.scheduleVision();
        }
      },
      onCreaturesChanged: () => state.creaturesTick.update((n) => n + 1),
      onCallingYou: () => this.callingYou(),
      onRevivifyChanged: () => state.revivifyTick.update((n) => n + 1),
      // A dead character lives again: its vitals are back (the character's page reads itself, on its own stream).
      onCharacterRevived: (characterId) => {
        void loader.readVitals(campaignId, generation);
        void loader.characterRevived(campaignId, generation, characterId);
      },
      onSpellCastsChanged: () => {
        state.castsTick.update((n) => n + 1);
        loader.loadMageArmor(campaignId, generation);
      },
      onGroupCheckChanged: () => state.groupCheckTick.update((n) => n + 1),
      onSessionMessage: (message) => void parts.conversation.receive(message),
      onMasterTyping: (seconds) => parts.conversation.masterTyping(seconds),
      // The sheet says the damage as the attack's, even when the attacker is hidden and has no line in the log.
      onAttackHit: (hit) => parts.combat.ownHit.set(ownHitOf(hit)),
      onHeldRollsChanged: () => state.heldRollsTick.update((n) => n + 1),
      // Someone at the table rolled (never a secret roll): the log gets it and the dice tumble.
      onFreeRollRolled: (roll) => parts.freeDice.heard(roll),
      onTableScreenChanged: (change) => parts.tableScreen.heard(change),
      onEnded: () => loader.ended(),
      onFatal: (kind) => {
        if (kind === 'signed-out') {
          services.auth.signIn(services.router.url);
        } else {
          state.phase.set('no-access');
        }
      },
    } satisfies Partial<LiveStreamHandlers>;
  }

  /** The map, the fog and the image on show. */
  private mapHandlers(generation: number) {
    const { state, parts, loader, view } = this;
    return {
      onCurrentMap: (mapId) => this.showMap(mapId),
      onMapChanged: (mapId) => this.mapChanged(mapId),
      onVisionChanged: (mapId) => {
        // The fog map's tokens, points and squares come from reads: read them again.
        if (mapId === state.currentMapId()) {
          void parts.mapState.refresh();
          this.loader.scheduleVision();
          // What a player sees changed: so may the log lines they read (a log is only read in an open combat).
          if (parts.combat.encounter()) {
            parts.combat.touchLog(true);
          }
          // In a combat the NPCs come with the combat, not with the map: a player whose sight changed (their own move, an
          // ally's, a light, a door) may now see one the last read left out. Only the combat read carries it (RN-10).
          if (!state.isMaster() && parts.combat.encounter() && !view.inTheatre()) {
            void loader.loadCombat(generation);
          }
        }
      },
      onTokenMoved: (move) => {
        this.loader.scheduleVision();
        void parts.trapBoard.tokensMoved();
        // A token the page doesn't know (a missed `map_changed`): read again.
        if (!parts.mapState.moveToken(move.mapId, move.characterId, move.xBp, move.yBp)) {
          void parts.mapState.refresh();
        }
      },
      onTrapNoticed: (notice) => void this.trapNoticed(notice.mapId, notice.pointId),
      onShownImage: (image) => this.shownImageChanged(image),
      onLeftImages: () => void loader.reloadLeftImages(),
    } satisfies Partial<LiveStreamHandlers>;
  }

  /** The combat on screen: the news is applied in place, or the combat is read again. */
  private combatHandlers(generation: number) {
    const { parts, loader, view } = this;
    return {
      onEncounterChanged: (change) => {
        // A combat without a map has no fog and no traps (RN-25): the hint says the mode, and nothing of the map is read for it.
        const noMap = change.mode === EncounterMode.THEATRE || (!change.mode && view.inTheatre());
        // Any change of the combat may change who sees whom (revision 0 is a hint with no number): read the vision too.
        if (!noMap) {
          this.loader.scheduleVision();
        }
        // A hint with no revision may mean a visibility change (a combatant shown or hidden): read the whole log.
        if (change.revision === 0 && parts.combat.encounter()) {
          parts.combat.touchLog(true);
        }
        // Read again only when the news is newer than the copy on screen. A hint with no revision
        // (0) is the server telling a player "read again" without counting: an opportunity offer
        // made to them, or any change on a fog map, where each player has a revision of their own
        // (ADR-0007). It is always read.
        const current = parts.combat.encounter();
        if (
          !current ||
          current.id !== change.encounterId ||
          change.revision === 0 ||
          change.revision > current.revision
        ) {
          void loader.loadCombat(generation);
        }
        // A trap that fired in the combat leaves damage that waits for the master.
        if (!noMap) {
          void parts.trapBoard.refreshDamages();
        }
      },
      // A window opened for this person: its prompt comes with the combat, read again like `encounter_changed`.
      onReactionWindowOpened: (opened) => {
        parts.combat.noteReactionOpened(opened.windowId);
        void loader.loadCombat(generation);
      },
      onReactionWindowClosed: (closed) => {
        // Closed by itself: the reason, in a status line, written for this person; the combat is read again.
        if (closed.closedByItself) {
          parts.combat.noteReactionClosed(closed.windowId, closed.text);
        }
        void loader.loadCombat(generation);
      },
      onTurnChanged: (turn) => {
        if (!parts.combat.applyTurn(turn)) {
          void loader.loadCombat(generation);
        }
      },
      onCombatantMoved: (move) => {
        if (!view.inTheatre()) {
          this.loader.scheduleVision();
          void parts.trapBoard.tokensMoved();
        }
        if (!parts.combat.applyMove(move)) {
          void loader.loadCombat(generation);
        }
      },
      onCombatLogChanged: (full) => {
        parts.combat.touchLog(full);
        if (!view.inTheatre()) {
          void parts.trapBoard.refreshDamages();
        }
      },
    } satisfies Partial<LiveStreamHandlers>;
  }

  /** The table's other news: XP, the scene, the notes, the puzzles. */
  private tableHandlers() {
    const { services, state, parts } = this;
    return {
      onPuzzleChanged: (id) => void parts.puzzles.changed(id),
      onXpChanged: () => {
        services.xpChanges.bump();
        // Converting a treasure to XP (and undoing it) changes its point on the map, with no map event.
        if (state.isMaster()) {
          void parts.mapState.refresh();
        }
      },
      onSceneChanged: () => void parts.scene.refresh(),
      onNotesChanged: () => {
        void parts.notes.refresh(true);
        // A cipher's key is a clue in the notes: the open puzzle reads its run again to learn the player found it.
        void parts.puzzles.refresh();
      },
      // The stage is part of the open scene: read it again (it names nobody).
      onStageChanged: () => void parts.scene.refresh(),
    } satisfies Partial<LiveStreamHandlers>;
  }

  /** The master's nudge: the same alert as a turn that comes, plus the line, which goes away by itself. */
  private callingYou(): void {
    const { state } = this;
    if (state.isMaster()) {
      return;
    }
    this.services.turnAlert.nudge('master');
    state.calling.set(true);
    clearTimeout(this.callingTimer);
    this.callingTimer = setTimeout(() => state.calling.set(false), CALLING_NOTICE_MS);
  }

  /** `current_map_changed`, or the master's own choice. */
  showMap(mapId: string | null): void {
    const { state } = this;
    this.mapEvents++;
    state.currentMapId.set(mapId);
    void this.parts.mapState.open(mapId);
    if (state.isMaster()) {
      void this.loader.reloadMaps();
    }
  }

  private mapChanged(mapId: string): void {
    const { state, parts } = this;
    // The damage that waits covers the whole campaign (ListTrapDamages): a trap on another map may have fired.
    void parts.trapBoard.refreshDamages();
    if (mapId === state.currentMapId()) {
      void parts.mapState.refresh();
      // Something about the traps may have changed: a firing, a notice, a damage that waits.
      void parts.trapBoard.refresh();
      this.loader.scheduleVision();
    }
    if (state.isMaster()) {
      void this.loader.reloadMaps();
    }
  }

  /** `trap_noticed` (E9-08 G): this player's character noticed a trap. The map is read again (the trap is on it now,
   * for them alone) and the toast names it. */
  private async trapNoticed(mapId: string, pointId: string): Promise<void> {
    const { state, parts } = this;
    if (mapId === state.currentMapId()) {
      await parts.mapState.refresh();
    }
    const name = parts.mapState.points().find((p) => p.id === pointId)?.name;
    parts.toasts.push(
      'visibility',
      'Você notou uma armadilha.',
      name ? `${name}, no mapa.` : 'Ela já aparece no seu mapa.',
    );
    void parts.trapBoard.refreshActivity();
  }

  /** `shown_image_changed`: the block appears, changes or goes away. */
  private shownImageChanged(image: ShownImageVm | null): void {
    const { state } = this;
    this.imageEvents++;
    if (!state.isMaster()) {
      const previous = state.shownImage();
      if (image) {
        state.shownNotice.set(`O mestre está mostrando ${image.name}.`);
      } else if (previous) {
        state.shownNotice.set('O mestre parou de mostrar a imagem.');
      }
    }
    // The switch is per image: a new image, or none, starts with it off.
    state.shownKeep.set(false);
    state.shownImage.set(image);
  }
}
