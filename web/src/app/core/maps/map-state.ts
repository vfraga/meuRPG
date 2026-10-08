import { signal } from '@angular/core';
import { Code, ConnectError } from '@connectrpc/connect';

import type {
  GetMapResponse,
  Map as MapMessage,
  MapPoint,
  MapToken,
} from '../../../gen/meurpg/maps/v1/maps_pb';

/**
 * - `idle`: no map to show (none chosen);
 * - `loading`: the first read of a map;
 * - `ready`: the map is on screen (a refetch keeps it there);
 * - `gone`: `not_found`: the map was deleted, or hidden from this player;
 * - `error`: the read failed for another reason.
 */
export type MapStatus = 'idle' | 'loading' | 'ready' | 'gone' | 'error';

/**
 * One open map's state (the session page and the map pages): the map, its
 * points and its tokens as signals, with the small updates the screens make
 * after their own calls and after the stream's events. Pure TypeScript, so
 * the rules (a stale answer never overwrites a newer one; `not_found` means
 * the player lost sight of the map) are tested without a DOM.
 */
export class MapState {
  readonly map = signal<MapMessage | null>(null);
  readonly points = signal<readonly MapPoint[]>([]);
  readonly tokens = signal<readonly MapToken[]>([]);
  readonly status = signal<MapStatus>('idle');

  private mapId: string | null = null;
  private generation = 0;
  /** Moves with each in-place edit (a stream event, the answer of a call): a read that began before it may carry the
   * state from before the edit, so it is dropped and the map is read once more. */
  private edits = 0;

  constructor(private readonly load: (mapId: string) => Promise<GetMapResponse>) {}

  /** Shows `mapId` (reading it), or nothing for `null`. */
  async open(mapId: string | null): Promise<void> {
    this.mapId = mapId;
    if (mapId === null) {
      this.generation++;
      this.clear('idle');
      return;
    }
    if (this.map()?.id !== mapId) {
      // A different map: don't leave the old one under the new name.
      this.clear('loading');
    }
    await this.read(mapId);
  }

  /** Reads the open map again (`map_changed`): the old one stays on screen
   * until the new answer arrives. */
  async refresh(): Promise<void> {
    if (this.mapId !== null) {
      await this.read(this.mapId);
    }
  }

  private async read(mapId: string): Promise<void> {
    const generation = ++this.generation;
    const edits = this.edits;
    try {
      const res = await this.load(mapId);
      if (generation !== this.generation) {
        return;
      }
      if (edits !== this.edits) {
        // Served before an edit that is already on screen: read again rather than undo it.
        return await this.read(mapId);
      }
      this.apply(res);
    } catch (err) {
      if (generation !== this.generation || edits !== this.edits) {
        return;
      }
      if (ConnectError.from(err).code === Code.NotFound) {
        this.clear('gone');
      } else if (this.status() !== 'ready') {
        this.status.set('error');
      }
      // A failed refetch of a map already on screen keeps what is there.
    }
  }

  apply(res: GetMapResponse): void {
    this.map.set(res.map ?? null);
    this.points.set(res.points);
    this.tokens.set(res.tokens);
    this.status.set(res.map ? 'ready' : 'gone');
  }

  private clear(status: MapStatus): void {
    this.map.set(null);
    this.points.set([]);
    this.tokens.set([]);
    this.status.set(status);
  }

  /** `token_moved`: moves a token without reading the map again. `false`
   * when the map is open but the token is not on it: the app missed a
   * `map_changed`, and should read the map again (maps.proto). A move on
   * another map is not this state's business (`true`). */
  moveToken(mapId: string, characterId: string, xBp: number, yBp: number): boolean {
    if (mapId !== this.map()?.id) {
      return true;
    }
    // A creature's token carries its owner's `character_id`: only the character's own token moves here.
    if (!this.tokens().some((t) => !t.creatureId && t.characterId === characterId)) {
      return false;
    }
    this.edits++;
    this.tokens.update((list) =>
      list.map((t) => (!t.creatureId && t.characterId === characterId ? { ...t, xBp, yBp } : t)),
    );
    return true;
  }

  /** Adds a point or replaces the one it names. A point of another map than the open one (a late answer after the
   * map changed) is not this state's. */
  upsertPoint(point: MapPoint): void {
    if (!this.isOpen(point.mapId)) {
      return;
    }
    this.edits++;
    this.points.update((list) =>
      list.some((p) => p.id === point.id)
        ? list.map((p) => (p.id === point.id ? point : p))
        : [...list, point],
    );
    this.touchCount();
  }

  removePoint(pointId: string): void {
    this.edits++;
    this.points.update((list) => list.filter((p) => p.id !== pointId));
    this.touchCount();
  }

  /** Adds a token or replaces the one it names. A creature's token carries its owner's `character_id`, so a token
   * is told apart by `tokenKey`: the creature's ID when it is one, the character's otherwise. */
  upsertToken(token: MapToken): void {
    if (!this.isOpen(token.mapId)) {
      return;
    }
    this.edits++;
    const key = tokenKey(token);
    this.tokens.update((list) =>
      list.some((t) => tokenKey(t) === key)
        ? list.map((t) => (tokenKey(t) === key ? token : t))
        : [...list, token],
    );
  }

  /** A creature's token goes off the map. */
  removeCreatureToken(creatureId: string): void {
    this.edits++;
    this.tokens.update((list) => list.filter((t) => t.creatureId !== creatureId));
  }

  /** A character's own token goes off the map (a creature's, which carries its owner's `character_id`, stays). */
  removeToken(characterId: string): void {
    this.edits++;
    this.tokens.update((list) => list.filter((t) => t.creatureId || t.characterId !== characterId));
  }

  setMap(map: MapMessage): void {
    this.edits++;
    this.map.set(map);
  }

  /** Whether a row that names its map (`map_id`) belongs to the map on screen; a row that names none does. */
  private isOpen(rowMapId: string): boolean {
    return !rowMapId || rowMapId === this.map()?.id;
  }

  private touchCount(): void {
    const map = this.map();
    if (map) {
      this.map.set({ ...map, pointCount: this.points().length });
    }
  }
}

/** What tells one token from another on a map: a creature's ID, or the character's (a creature's `character_id` is its owner's). */
export function tokenKey(token: {
  readonly characterId: string;
  readonly creatureId?: string;
}): string {
  return token.creatureId || token.characterId;
}
