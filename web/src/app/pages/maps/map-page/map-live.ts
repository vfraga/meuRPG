import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';

import { PlayService } from '../../../../gen/meurpg/play/v1/play_pb';
import { CONNECT_TRANSPORT } from '../../../core/connect/transport';
import { classifyLiveError } from '../../../core/play/live-error';
import { LiveStream, type StreamStatus } from '../../../core/play/live-stream';
import type { LiveErrorKind, LiveEventVm } from '../../../core/play/live-session.types';

/** A burst of news (a token dragged, a door and its vision) makes one read. */
export const MAP_LIVE_DEBOUNCE_MS = 150;

/**
 * The campaign's live stream (`WatchGameSession`) for the map page: only the events that can change what a map
 * shows are translated; everything else counts as a heartbeat (proof that the stream is alive). The same stream the
 * session page opens, with the same per-member filtering on the server (RN-10): this adds no read of its own.
 */
@Injectable({ providedIn: 'root' })
export class MapLiveClient {
  private readonly play = createClient(PlayService, inject(CONNECT_TRANSPORT));

  async *watch(campaignId: string, signal: AbortSignal): AsyncIterable<LiveEventVm> {
    for await (const res of this.play.watchGameSession({ campaignId }, { signal })) {
      const e = res.event;
      switch (e.case) {
        case 'ready':
          yield { kind: 'ready' };
          break;
        case 'sessionEnded':
          yield { kind: 'ended' };
          break;
        case 'currentMapChanged':
          yield { kind: 'currentMap', mapId: e.value.mapId || null };
          break;
        case 'mapChanged':
          yield { kind: 'mapChanged', mapId: e.value.mapId };
          break;
        case 'tokenMoved':
          yield {
            kind: 'tokenMoved',
            mapId: e.value.mapId,
            characterId: e.value.characterId,
            xBp: e.value.xBp,
            yBp: e.value.yBp,
          };
          break;
        case 'visionChanged':
          yield { kind: 'visionChanged', mapId: e.value.mapId };
          break;
        case 'encounterChanged':
          yield {
            kind: 'encounterChanged',
            encounterId: e.value.encounterId,
            revision: e.value.revision,
            mode: e.value.mode,
          };
          break;
        case 'combatantMoved':
          yield {
            kind: 'combatantMoved',
            encounterId: e.value.encounterId,
            combatantId: e.value.combatantId,
            col: e.value.col,
            row: e.value.row,
          };
          break;
        case 'trapNoticed':
          yield { kind: 'trapNoticed', mapId: e.value.mapId, pointId: e.value.pointId };
          break;
        case 'xpChanged':
          yield { kind: 'xpChanged' };
          break;
        default:
          yield { kind: 'heartbeat' };
      }
    }
  }

  classifyError(err: unknown): LiveErrorKind {
    return classifyLiveError(err);
  }
}

/** What the page lets `MapLive` do. */
export interface MapLiveHost {
  /** The map on screen, or `null`. */
  mapId(): string | null;
  /** Whether the person is the campaign's master. */
  isMaster(): boolean;
  /** Reads the map again (the page's map state). */
  refresh(): void;
  /** What the player sees of a fog map may have changed: reads the vision again. */
  visionChanged(): void;
  /** Moves a token in place; `false` when the token is not on the map (a missed `map_changed`). */
  moveToken(mapId: string, characterId: string, xBp: number, yBp: number): boolean;
  /** The master's flags (the combat on this map, the open session) and the map list changed. */
  flagsChanged(): void;
  mapsChanged(): void;
}

export interface MapLiveOptions {
  open(campaignId: string, signal: AbortSignal): AsyncIterable<LiveEventVm>;
  classify(err: unknown): LiveErrorKind;
  document: Document;
  host: MapLiveHost;
  debounceMs?: number;
}

/**
 * Keeps the map page current while the campaign has an open session: it listens to the campaign's live stream and
 * reads the map again (once per burst) on the news that can change it. Without an open session the stream ends by
 * itself (`NO_OPEN_SESSION`) and the page stays as it was; `ensure()` tries again when the person comes back to the
 * window. The stream class does the rest: reconnection with backoff, the dead-stream timer and the hidden tab.
 */
export class MapLive {
  private stream: LiveStream | null = null;
  private campaignId = '';
  private everReady = false;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private pending = { map: false, vision: false, flags: false, maps: false };

  constructor(private readonly options: MapLiveOptions) {}

  get status(): StreamStatus | null {
    return this.stream?.status() ?? null;
  }

  /** The stream for `campaignId`: the same campaign keeps its stream (even one that is reconnecting), another replaces it. */
  start(campaignId: string): void {
    if (this.stream && this.campaignId === campaignId && this.stream.status() !== 'closed') {
      return;
    }
    this.stop();
    this.campaignId = campaignId;
    this.everReady = false;
    const host = this.options.host;
    const onThisMap = (mapId: string) => mapId === host.mapId();
    const stream: LiveStream = new LiveStream({
      open: (signal) => this.options.open(campaignId, signal),
      classify: (err) => this.options.classify(err),
      document: this.options.document,
      handlers: {
        onReady: () => {
          // The page has just read the map; a reconnection may have missed news, so it reads again.
          if (this.everReady) {
            this.schedule({ map: true, flags: host.isMaster() });
          }
          this.everReady = true;
        },
        onVitals: () => undefined,
        onMapChanged: (mapId) => {
          if (onThisMap(mapId)) {
            this.schedule({ map: true });
          }
        },
        onVisionChanged: (mapId) => {
          if (onThisMap(mapId)) {
            this.schedule({ map: true });
          }
        },
        onTokenMoved: (move) => {
          if (!onThisMap(move.mapId)) {
            return;
          }
          if (host.moveToken(move.mapId, move.characterId, move.xBp, move.yBp)) {
            // The token is in its place; a fog map's vision moved with it.
            this.schedule({ vision: true });
          } else {
            this.schedule({ map: true });
          }
        },
        onTrapNoticed: (notice) => {
          if (onThisMap(notice.mapId)) {
            this.schedule({ map: true });
          }
        },
        // The events of a combat do not name the map: read it again, which is cheap and safe.
        onEncounterChanged: () => this.schedule({ map: true, flags: host.isMaster() }),
        onCombatantMoved: () => this.schedule({ map: true }),
        onCurrentMap: () => this.schedule({ map: true, maps: true, flags: host.isMaster() }),
        // Turning a treasure into XP changes its point, with no map event (the master's).
        onXpChanged: () => {
          if (host.isMaster()) {
            this.schedule({ map: true });
          }
        },
        onEnded: () => undefined,
        onFatal: () => undefined,
      },
    });
    this.stream = stream;
    stream.start();
  }

  /** Opens the stream again if it ended (no session then, a session now). */
  ensure(): void {
    if (this.campaignId !== '' && this.stream?.status() === 'closed') {
      this.start(this.campaignId);
    }
  }

  stop(): void {
    this.stream?.stop();
    this.stream = null;
    if (this.timer !== undefined) {
      clearTimeout(this.timer);
      this.timer = undefined;
    }
    this.pending = { map: false, vision: false, flags: false, maps: false };
  }

  private schedule(what: {
    map?: boolean;
    vision?: boolean;
    flags?: boolean;
    maps?: boolean;
  }): void {
    this.pending.map ||= what.map === true;
    this.pending.vision ||= what.map === true || what.vision === true;
    this.pending.flags ||= what.flags === true;
    this.pending.maps ||= what.maps === true;
    if (this.timer !== undefined) {
      return;
    }
    this.timer = setTimeout(() => {
      this.timer = undefined;
      const pending = this.pending;
      this.pending = { map: false, vision: false, flags: false, maps: false };
      const host = this.options.host;
      if (pending.map) {
        host.refresh();
      }
      if (pending.vision) {
        host.visionChanged();
      }
      if (pending.flags) {
        host.flagsChanged();
      }
      if (pending.maps) {
        host.mapsChanged();
      }
    }, this.options.debounceMs ?? MAP_LIVE_DEBOUNCE_MS);
  }
}
