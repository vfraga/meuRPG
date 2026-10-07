// Finding U11-01: PaintQueue.clear() while a send is in flight lets drain() shift()/overwrite the new head.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';

import { MapLayer } from '../../../gen/meurpg/maps/v1/maps_pb';
import { PaintQueue, type PaintTarget } from './paint-queue';

describe('Review11 U11-01: clear() during an in-flight send', () => {
  const T: PaintTarget = { campaignId: 'c', mapId: 'a' };
  let sent: { value: number; squares: { col: number; row: number }[] }[];
  let resolvers: { ok: () => void; fail: (e: unknown) => void }[];
  let queue: PaintQueue;

  beforeEach(() => {
    vi.useFakeTimers();
    sent = [];
    resolvers = [];
    queue = new PaintQueue(
      (_t, _l, value, squares) =>
        new Promise<void>((ok, fail) => {
          sent.push({ value, squares: [...squares] });
          resolvers.push({ ok, fail });
        }),
      100,
    );
  });
  afterEach(() => vi.useRealTimers());

  it('does not drop strokes painted after clear() when the old send resolves', async () => {
    queue.add(T, MapLayer.WALL, 1, [{ col: 1, row: 1 }]);
    void queue.flush();
    expect(sent).toHaveLength(1);
    queue.clear();
    queue.add(T, MapLayer.WALL, 2, [{ col: 5, row: 5 }]);
    resolvers[0].ok();
    await vi.advanceTimersByTimeAsync(0);
    // The new stroke must still be pending (or already sent), never silently lost.
    const lost = queue.status() === 'saved' && !sent.some((s) => s.value === 2);
    expect(lost, 'new stroke dropped and status says saved').toBe(false);
  });

  it('does not resend old-grid squares after clear() when the old send fails transiently', async () => {
    queue.add(T, MapLayer.WALL, 1, [{ col: 1, row: 1 }]);
    void queue.flush();
    queue.clear();
    resolvers[0].fail(new ConnectError('down', Code.Unavailable));
    await vi.advanceTimersByTimeAsync(0);
    void queue.retry();
    await vi.advanceTimersByTimeAsync(0);
    expect(sent.filter((s) => s.value === 1)).toHaveLength(1);
  });
});
