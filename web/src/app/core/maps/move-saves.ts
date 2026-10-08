/** A position on a map, in basis points. */
export interface MapPosition {
  readonly xBp: number;
  readonly yBp: number;
}

/** What one move saves, and what to do if saving it fails. */
export interface MoveSave {
  /** Sends the position to the server. */
  readonly save: (to: MapPosition) => Promise<unknown>;
  /** Puts the item back where the server has it, and says what failed. */
  readonly failed: (saved: MapPosition, err: unknown) => void;
}

interface Pending {
  saved: MapPosition;
  next: { to: MapPosition; handlers: MoveSave } | null;
}

/**
 * Saves the moves of each item (a point, a token) one at a time, in order.
 *
 * Two quick moves of the same token would otherwise race: both requests
 * write the same row, and a CockroachDB retry can commit the older one last,
 * leaving it on the server and on every other screen at the table. While a
 * save is in flight, only the latest move waits; the ones in between are
 * skipped. If a save fails, the waiting move is dropped too, and `failed`
 * gets the last position the server confirmed, so the screen and the server
 * agree again.
 */
export class MoveSaves {
  private readonly pending = new Map<string, Pending>();

  /** Whether a save of `key` is in flight or waiting: what the screen shows for it is ahead of the server. */
  isPending(key: string): boolean {
    return this.pending.has(key);
  }

  /**
   * Saves the move of `key` (one key per item and map) from `from`, where
   * the screen showed it before this move, to `to`. A move that waits
   * behind another resolves at once; the first one resolves when no save
   * for `key` is left.
   */
  async move(key: string, from: MapPosition, to: MapPosition, handlers: MoveSave): Promise<void> {
    const waiting = this.pending.get(key);
    if (waiting) {
      waiting.next = { to, handlers };
      return;
    }
    const entry: Pending = { saved: { xBp: from.xBp, yBp: from.yBp }, next: { to, handlers } };
    this.pending.set(key, entry);
    try {
      while (entry.next) {
        const { to: target, handlers: current } = entry.next;
        entry.next = null;
        try {
          await current.save(target);
          entry.saved = target;
        } catch (err) {
          entry.next = null;
          current.failed(entry.saved, err);
          return;
        }
      }
    } finally {
      this.pending.delete(key);
    }
  }
}
