import { signal } from '@angular/core';
import { Code, ConnectError } from '@connectrpc/connect';

import type { MapLayer } from '../../../gen/meurpg/maps/v1/maps_pb';
import type { Square } from '../combat/combat-grid';

/** The server takes at most this many squares a call (`PaintMapCells`). */
export const MAX_PAINT_BATCH = 400;

/** `saved`: nothing waits ("Tudo salvo"); `saving`: strokes wait or are on their way; `error`: a batch was refused. */
export type PaintSaveStatus = 'saved' | 'saving' | 'error';

/** The map a stroke was made on, kept with it: the editor may be on another map by the time it is sent. */
export interface PaintTarget {
  readonly campaignId: string;
  readonly mapId: string;
}

interface Batch {
  readonly target: PaintTarget;
  readonly layer: MapLayer;
  readonly value: number;
  readonly squares: Map<number, Square>;
}

/** Whether trying again can help: the server did not answer, or the call was cut. Anything else (the map is gone, the caller may not
 * paint, the map has no grid) is a refusal that no retry changes. A failure that is not a Connect error is a network one. */
export function isTransient(err: unknown): boolean {
  const code = ConnectError.from(err, Code.Unavailable).code;
  return code === Code.Unavailable || code === Code.DeadlineExceeded || code === Code.Aborted;
}

/**
 * The strokes of the master's painting, sent in batches (MR-034, E9-01): every square of a drag joins the batch of its
 * layer, value and **map**, and the batch goes out a moment later (or at once on `flush()`, at the end of a stroke), at most
 * 400 squares a call and one call at a time, in the order of the strokes, so the last write wins as the server's rule says.
 * `status` is the "Tudo salvo" tag. A batch that failed for a reason a retry can fix (the server did not answer) stays at the
 * head of the queue and `retry()` sends it again; any other refusal says why (`failure`) and **drops what waits**: those strokes
 * were made on a screen the server disagrees with, so the editor reads the layers again (`refused`). Plain TypeScript with a
 * timer, so the batching is tested with fake time.
 */
export class PaintQueue {
  readonly status = signal<PaintSaveStatus>('saved');
  /** Why the last batch was refused; `null` otherwise. */
  readonly failure = signal<unknown>(null);
  /** Whether "Tentar de novo" can help with `failure`. */
  readonly retryable = signal(false);
  /** Goes up every time a refusal dropped strokes: the editor reads what the server has again. */
  readonly refused = signal(0);

  private readonly batches: Batch[] = [];
  private timer: ReturnType<typeof setTimeout> | undefined;
  private running: Promise<void> | null = null;
  private drainedCallbacks: (() => void)[] = [];

  constructor(
    private readonly send: (
      target: PaintTarget,
      layer: MapLayer,
      value: number,
      squares: readonly Square[],
    ) => Promise<void>,
    private readonly delayMs = 150,
  ) {}

  /** Whether strokes wait or are being saved: the editor does not read the layers again meanwhile. */
  get busy(): boolean {
    return this.batches.length > 0 || this.running !== null;
  }

  /** Runs `f` the next time the queue is empty and quiet (at once if it is). */
  whenIdle(f: () => void): void {
    if (this.busy) {
      this.drainedCallbacks.push(f);
    } else {
      f();
    }
  }

  add(target: PaintTarget, layer: MapLayer, value: number, squares: readonly Square[]): void {
    if (squares.length === 0) {
      return;
    }
    const last = this.batches[this.batches.length - 1];
    const batch =
      last &&
      last.layer === layer &&
      last.value === value &&
      last.target.mapId === target.mapId &&
      last.target.campaignId === target.campaignId &&
      !(this.running && this.batches.length === 1)
        ? last
        : this.openBatch(target, layer, value);
    for (const s of squares) {
      batch.squares.set(key(s), s);
    }
    this.status.set('saving');
    this.failure.set(null);
    clearTimeout(this.timer);
    this.timer = setTimeout(() => void this.flush(), this.delayMs);
  }

  private openBatch(target: PaintTarget, layer: MapLayer, value: number): Batch {
    const batch: Batch = { target, layer, value, squares: new Map() };
    this.batches.push(batch);
    return batch;
  }

  /** Throws away what waits (the grid changed: those squares are not on the map any more). */
  clear(): void {
    clearTimeout(this.timer);
    this.batches.length = 0;
    this.failure.set(null);
    this.retryable.set(false);
    if (this.running === null) {
      this.status.set('saved');
    }
  }

  /** Sends everything that waits, now. Resolves when the queue is empty or a batch was refused. */
  flush(): Promise<void> {
    clearTimeout(this.timer);
    this.running ??= this.drain().finally(() => {
      this.running = null;
      if (!this.busy) {
        const callbacks = this.drainedCallbacks.splice(0);
        callbacks.forEach((f) => f());
      }
    });
    return this.running;
  }

  /** Sends the refused batch again (only a transient failure keeps one). */
  retry(): Promise<void> {
    if (this.status() === 'error') {
      this.status.set('saving');
      this.failure.set(null);
    }
    return this.flush();
  }

  private async drain(): Promise<void> {
    while (this.batches.length > 0) {
      // Held by reference: `clear()` may empty the queue while a call is in flight, and then the head is another batch.
      const batch = this.batches[0];
      const squares = [...batch.squares.values()];
      for (let i = 0; i < squares.length && this.batches[0] === batch; i += MAX_PAINT_BATCH) {
        try {
          await this.send(
            batch.target,
            batch.layer,
            batch.value,
            squares.slice(i, i + MAX_PAINT_BATCH),
          );
        } catch (err) {
          if (this.fail(err, batch, squares, i)) {
            return;
          }
          break;
        }
      }
      if (this.batches[0] === batch) {
        this.batches.shift();
      }
    }
    this.retryable.set(false);
    this.status.set('saved');
  }

  /** A send failed: records why and what stays queued. Returns whether the drain stops (`false`: the batch was thrown away and the failure is not about what waits). */
  private fail(err: unknown, batch: Batch, squares: readonly Square[], from: number): boolean {
    const cleared = this.batches[0] !== batch;
    if (cleared && !isTransient(err)) {
      return false;
    }
    this.failure.set(err);
    this.status.set('error');
    if (isTransient(err)) {
      if (!cleared) {
        // What was sent stays sent: the head keeps only what is left.
        const left = new Map<number, Square>();
        for (const s of squares.slice(from)) {
          left.set(key(s), s);
        }
        this.batches[0] = { ...batch, squares: left };
      }
      this.retryable.set(true);
    } else {
      this.batches.length = 0;
      this.retryable.set(false);
      this.refused.update((n) => n + 1);
      this.drainedCallbacks.length = 0;
    }
    return true;
  }
}

function key(s: Square): number {
  return s.row * 65_536 + s.col;
}
