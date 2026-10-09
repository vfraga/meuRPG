import { signal } from '@angular/core';

import type { CombatantMove, TurnChange } from '../../core/combat/combat-state';
import { LiveErrorKind, LiveEventVm, ShownImageVm, VitalsVm } from './live-session.types';

/**
 * Where the stream stands:
 * - `connecting`: the first connection, before `ready`;
 * - `live`: `ready` arrived and messages keep coming;
 * - `reconnecting`: it dropped and a new try is scheduled or running;
 * - `paused`: the tab is hidden, so the stream is closed on purpose and
 *   reopens when the tab is visible again (ADR-0005's cost guard);
 * - `closed`: over for good (the session ended, no access, signed out, or
 *   the page left).
 */
export type StreamStatus = 'connecting' | 'live' | 'reconnecting' | 'paused' | 'closed';

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
  /** `turn_changed`: applied in place, per audience. */
  onTurnChanged?(turn: TurnChange): void;
  /** `combatant_moved`: applied in place. */
  onCombatantMoved?(move: CombatantMove): void;
  /** `combat_log_changed`: the log has a new entry, read it again. */
  onCombatLogChanged?(): void;
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
  /** `inventory_changed` (W7-I): a character's inventory changed; read it again. */
  onInventoryChanged?(characterId: string): void;
  /** `content_changed` (10.1d): the table's content changed; a screen that shows the catalog reads it again. */
  onContentChanged?(): void;
  /** `puzzle_changed` (MR-038): a puzzle changed; the page reads it again (the server already throttles the hint). */
  onPuzzleChanged?(puzzleId: string): void;
  /** `session_ended`, or `NO_OPEN_SESSION` when (re)connecting. */
  onEnded(): void;
  /** `not_found` or `unauthenticated`: no reconnecting. */
  onFatal(kind: 'no-access' | 'signed-out'): void;
}

export interface LiveStreamOptions {
  open(signal: AbortSignal): AsyncIterable<LiveEventVm>;
  classify(err: unknown): LiveErrorKind;
  handlers: LiveStreamHandlers;
  document: Document;
  /** For tests; `Math.random` by default. */
  random?: () => number;
}

/** No message for this long (the server sends a heartbeat every 25 s):
 * the stream is dead, even if the connection looks open. */
export const DEAD_STREAM_MS = 60_000;
/** Hidden this long, the tab closes its stream. */
export const HIDDEN_CLOSE_MS = 2 * 60_000;
/** Backoff: 1 s, 2 s, 4 s … up to 30 s. */
export const BACKOFF_MAX_MS = 30_000;

/** The wait before reconnection attempt `attempt` (0 first): doubling from
 * 1 s up to 30 s, each with ±20 % jitter so a table of phones doesn't
 * reconnect in lockstep after a server restart. */
export function backoffDelay(attempt: number, random: () => number = Math.random): number {
  const base = Math.min(BACKOFF_MAX_MS, 1000 * 2 ** attempt);
  return Math.min(BACKOFF_MAX_MS, Math.round(base * (0.8 + 0.4 * random())));
}

/**
 * The live session's stream, with ADR-0005's client rules
 * (docs/architecture.md#live-session):
 *
 * - `ready` first; the page reads the snapshot on each one;
 * - reconnects after any end that is not final, with backoff and jitter,
 *   and only while the tab is visible;
 * - closes itself after 2 minutes hidden, and reopens (with a new
 *   snapshot) when the tab is visible again;
 * - 60 seconds without any message is a dead stream: close and reconnect;
 * - `not_found` and `unauthenticated` end it without reconnecting;
 *   `NO_OPEN_SESSION` and `session_ended` end it as "the session ended".
 *
 * Plain TypeScript with signals, so its timing is tested with fake timers.
 */
export class LiveStream {
  readonly status = signal<StreamStatus>('connecting');
  /** When the last message (any, heartbeats too) arrived. */
  readonly lastMessageAt = signal<Date | null>(null);

  private readonly random: () => number;
  private controller: AbortController | null = null;
  private attempt = 0;
  private restarts = 0;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private deadTimer: ReturnType<typeof setTimeout> | null = null;
  private hiddenTimer: ReturnType<typeof setTimeout> | null = null;
  private readonly onVisibility = () => this.visibilityChanged();

  constructor(private readonly options: LiveStreamOptions) {
    this.random = options.random ?? Math.random;
  }

  start(): void {
    this.options.document.addEventListener('visibilitychange', this.onVisibility);
    if (this.hidden()) {
      // Opened in a background tab: wait until it is looked at.
      this.status.set('paused');
      return;
    }
    void this.connect();
  }

  /** Closes for good (the page is leaving, or the session is over). */
  stop(): void {
    this.status.set('closed');
    this.options.document.removeEventListener('visibilitychange', this.onVisibility);
    this.clearTimers();
    this.abortCurrent();
  }

  /** Drops the current stream and connects again after the backoff: the
   * page asks this when a snapshot failed, so the next `ready` reads a new
   * one. Each restart in a row waits longer (a `ready` alone doesn't reset
   * the wait here, or a failing snapshot would retry every second), until
   * the page says a snapshot worked (`confirmHealthy`). */
  restart(): void {
    if (this.status() === 'closed') {
      return;
    }
    this.restarts++;
    this.attempt = Math.max(this.attempt, this.restarts);
    this.abortCurrent();
    this.scheduleReconnect();
  }

  /** The page read a snapshot: the next restart waits the shortest time. */
  confirmHealthy(): void {
    this.restarts = 0;
  }

  private async connect(): Promise<void> {
    this.clearRetry();
    if (this.status() === 'closed') {
      return;
    }
    if (this.hidden()) {
      this.status.set('paused');
      return;
    }
    const controller = new AbortController();
    this.controller = controller;
    this.armDeadTimer();
    try {
      for await (const event of this.options.open(controller.signal)) {
        if (controller !== this.controller) {
          return; // replaced: a newer connection owns the page now
        }
        this.lastMessageAt.set(new Date());
        this.armDeadTimer();
        switch (event.kind) {
          case 'ready':
            this.attempt = 0;
            this.status.set('live');
            this.options.handlers.onReady();
            break;
          case 'vitals':
            this.options.handlers.onVitals(event.vitals);
            break;
          case 'currentMap':
            this.options.handlers.onCurrentMap?.(event.mapId);
            break;
          case 'mapChanged':
            this.options.handlers.onMapChanged?.(event.mapId);
            break;
          case 'tokenMoved':
            this.options.handlers.onTokenMoved?.(event);
            break;
          case 'visionChanged':
            this.options.handlers.onVisionChanged?.(event.mapId);
            break;
          case 'leftImages':
            this.options.handlers.onLeftImages?.();
            break;
          case 'shownImage':
            this.options.handlers.onShownImage?.(event.image);
            break;
          case 'encounterChanged':
            this.options.handlers.onEncounterChanged?.(event);
            break;
          case 'turnChanged':
            this.options.handlers.onTurnChanged?.(event);
            break;
          case 'combatantMoved':
            this.options.handlers.onCombatantMoved?.(event);
            break;
          case 'combatLogChanged':
            this.options.handlers.onCombatLogChanged?.();
            break;
          case 'xpChanged':
            this.options.handlers.onXpChanged?.();
            break;
          case 'sceneChanged':
          case 'sceneCheckRolled':
            this.options.handlers.onSceneChanged?.();
            break;
          case 'notesChanged':
            this.options.handlers.onNotesChanged?.();
            break;
          case 'stageChanged':
            this.options.handlers.onStageChanged?.();
            break;
          case 'trapNoticed':
            this.options.handlers.onTrapNoticed?.(event);
            break;
          case 'creaturesChanged':
            this.options.handlers.onCreaturesChanged?.();
            break;
          case 'inventoryChanged':
            this.options.handlers.onInventoryChanged?.(event.characterId);
            break;
          case 'contentChanged':
            this.options.handlers.onContentChanged?.();
            break;
          case 'puzzleChanged':
            this.options.handlers.onPuzzleChanged?.(event.puzzleId);
            break;
          case 'ended':
            this.stop();
            this.options.handlers.onEnded();
            return;
          case 'heartbeat':
            break;
        }
      }
      if (controller !== this.controller) {
        return;
      }
      // Ended without an error (the 30-minute cap, a server restart).
      this.controller = null;
      this.scheduleReconnect();
    } catch (err) {
      if (controller !== this.controller) {
        return; // we aborted it ourselves (dead stream, hidden tab, stop)
      }
      this.controller = null;
      switch (this.options.classify(err)) {
        case 'no-access':
          this.stop();
          this.options.handlers.onFatal('no-access');
          return;
        case 'signed-out':
          this.stop();
          this.options.handlers.onFatal('signed-out');
          return;
        case 'no-session':
          this.stop();
          this.options.handlers.onEnded();
          return;
        default:
          this.scheduleReconnect();
      }
    }
  }

  private scheduleReconnect(): void {
    this.clearDeadTimer();
    if (this.status() === 'closed') {
      return;
    }
    if (this.hidden()) {
      this.status.set('paused');
      return;
    }
    this.status.set('reconnecting');
    const delay = backoffDelay(this.attempt, this.random);
    this.attempt++;
    this.clearRetry();
    this.retryTimer = setTimeout(() => void this.connect(), delay);
  }

  private visibilityChanged(): void {
    if (this.status() === 'closed') {
      return;
    }
    if (this.hidden()) {
      // Keep the stream for 2 minutes (a quick look at another app), then
      // close it: a forgotten tab holds nothing open.
      this.clearHiddenTimer();
      this.hiddenTimer = setTimeout(() => {
        this.hiddenTimer = null;
        this.abortCurrent();
        this.clearRetry();
        this.clearDeadTimer();
        this.status.set('paused');
      }, HIDDEN_CLOSE_MS);
      return;
    }
    this.clearHiddenTimer();
    if (this.status() === 'paused') {
      // Back in view: connect now, not after a backoff.
      this.attempt = 0;
      this.status.set('reconnecting');
      void this.connect();
    }
  }

  private armDeadTimer(): void {
    this.clearDeadTimer();
    this.deadTimer = setTimeout(() => {
      this.deadTimer = null;
      this.abortCurrent();
      this.scheduleReconnect();
    }, DEAD_STREAM_MS);
  }

  private abortCurrent(): void {
    const controller = this.controller;
    this.controller = null;
    controller?.abort();
  }

  private hidden(): boolean {
    return this.options.document.visibilityState === 'hidden';
  }

  private clearTimers(): void {
    this.clearRetry();
    this.clearDeadTimer();
    this.clearHiddenTimer();
  }

  private clearRetry(): void {
    if (this.retryTimer !== null) {
      clearTimeout(this.retryTimer);
      this.retryTimer = null;
    }
  }

  private clearDeadTimer(): void {
    if (this.deadTimer !== null) {
      clearTimeout(this.deadTimer);
      this.deadTimer = null;
    }
  }

  private clearHiddenTimer(): void {
    if (this.hiddenTimer !== null) {
      clearTimeout(this.hiddenTimer);
      this.hiddenTimer = null;
    }
  }
}
