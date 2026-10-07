// Finding U14-1 in review/unit-14-web-maps.md
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';

import { MapLayer } from '../../../gen/meurpg/maps/v1/maps_pb';
import { PaintQueue, type PaintTarget } from './paint-queue';
import { PaintSession } from './paint-session';

describe('Review14 U14-1: PaintQueue.clear() while a send is in flight', () => {
  const A: PaintTarget = { campaignId: 'c', mapId: 'a' };
  const B: PaintTarget = { campaignId: 'c', mapId: 'b' };
  let sent: { mapId: string; n: number }[];
  let pending: { resolve: () => void; reject: (e: unknown) => void }[];
  let queue: PaintQueue;

  beforeEach(() => {
    vi.useFakeTimers();
    sent = [];
    pending = [];
    queue = new PaintQueue(
      (t, _l, _v, sq) =>
        new Promise<void>((resolve, reject) => {
          sent.push({ mapId: t.mapId, n: sq.length });
          pending.push({ resolve, reject });
        }),
      100,
    );
  });
  afterEach(() => vi.useRealTimers());

  it('still sends a stroke added after clear() when the in-flight send succeeds', async () => {
    queue.add(A, MapLayer.WALL, 1, [{ col: 0, row: 0 }]);
    const flushing = queue.flush();
    expect(sent).toEqual([{ mapId: 'a', n: 1 }]);
    queue.clear();
    queue.add(B, MapLayer.WALL, 1, [{ col: 1, row: 1 }]);
    pending[0].resolve();
    await vi.advanceTimersByTimeAsync(0);
    // let the queue go on, as the 150 ms timer / next flush would
    await vi.advanceTimersByTimeAsync(500);
    await flushing;
    // correct behaviour: the new stroke goes to its own map before "saved"
    expect(sent.map((s) => s.mapId)).toEqual(['a', 'b']);
    if (pending[1]) pending[1].resolve();
    await vi.advanceTimersByTimeAsync(0);
    expect(queue.status()).toBe('saved');
  });

  it('does not say "saved" while the new stroke has not been sent', async () => {
    queue.add(A, MapLayer.WALL, 1, [{ col: 0, row: 0 }]);
    const flushing = queue.flush();
    queue.clear();
    queue.add(B, MapLayer.WALL, 1, [{ col: 1, row: 1 }]);
    pending[0].resolve();
    await flushing;
    expect(sent.length === 2 || queue.status() !== 'saved').toBe(true);
  });

  it('keeps the new batch (not the old one) when the in-flight send fails transiently', async () => {
    queue.add(A, MapLayer.WALL, 1, [{ col: 0, row: 0 }]);
    const flushing = queue.flush();
    queue.clear();
    queue.add(B, MapLayer.WALL, 1, [{ col: 1, row: 1 }]);
    pending[0].reject(new ConnectError('down', Code.Unavailable));
    await flushing;
    const retrying = queue.retry();
    await vi.advanceTimersByTimeAsync(0);
    pending.slice(1).forEach((p) => p.resolve());
    await retrying;
    expect(sent.map((s) => s.mapId)).toContain('b');
    expect(sent.map((s) => s.mapId).slice(1)).not.toContain('a');
  });
});

describe('Review14 U14-1: PaintSession.open with a changed grid while a batch is in flight', () => {
  it('sends a stroke made on the new grid', async () => {
    vi.useFakeTimers();
    try {
      const paints: { mapId: string; n: number }[] = [];
      const resolvers: (() => void)[] = [];
      const api = {
        layers: vi.fn(async () => ({
          gridColumns: 10,
          gridRows: 10,
          difficultTerrain: new Uint8Array(),
          wall: new Uint8Array(),
          cover: new Uint8Array(),
        }) as never),
        paint: vi.fn(
          (_c: string, mapId: string, _l: MapLayer, _v: number, sq: readonly unknown[]) =>
            new Promise<unknown>((resolve) => {
              paints.push({ mapId, n: sq.length });
              resolvers.push(() => resolve(undefined));
            }),
        ),
      };
      const session = new PaintSession(api as never);
      await session.open('c', 'm', 10, 1);
      session.queue.add({ campaignId: 'c', mapId: 'm' }, MapLayer.WALL, 1, [{ col: 0, row: 0 }]);
      void session.queue.flush(); // in flight
      expect(paints.length).toBe(1);
      // second tab changed the grid; the editor re-reads the map and calls open with new columns
      await session.open('c', 'm', 20, 2);
      session.queue.add({ campaignId: 'c', mapId: 'm' }, MapLayer.WALL, 1, [{ col: 15, row: 3 }]);
      resolvers[0]();
      await vi.advanceTimersByTimeAsync(1000);
      if (resolvers[1]) resolvers[1]();
      await vi.advanceTimersByTimeAsync(1000);
      expect(paints.length).toBe(2);
    } finally {
      vi.useRealTimers();
    }
  });
});
