import { describe, expect, it, vi } from 'vitest';

import { MapLayer } from '../../../gen/meurpg/maps/v1/maps_pb';
import { PaintSession } from './paint-session';

describe('PaintSession', () => {
  it('sends a stroke made on the new grid while a batch of the old one is in flight', async () => {
    vi.useFakeTimers();
    try {
      const paints: { mapId: string; n: number }[] = [];
      const resolvers: (() => void)[] = [];
      const api = {
        layers: vi.fn(
          async () =>
            ({
              gridColumns: 10,
              gridRows: 10,
              difficultTerrain: new Uint8Array(),
              wall: new Uint8Array(),
              cover: new Uint8Array(),
            }) as never,
        ),
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
