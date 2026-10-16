import { type Mock, afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { LiveEventVm } from '../../../core/play/live-session.types';
import { MAP_LIVE_DEBOUNCE_MS, MapLive, type MapLiveHost } from './map-live';

/** A stream the test pushes events into; aborting its signal ends it. */
class FakeFeed {
  opened = 0;
  aborted = 0;
  private queue: LiveEventVm[] = [];
  private wake: (() => void) | null = null;

  open = (_campaignId: string, signal: AbortSignal): AsyncIterable<LiveEventVm> => {
    this.opened++;
    signal.addEventListener('abort', () => {
      this.aborted++;
      this.wake?.();
    });
    // eslint-disable-next-line @typescript-eslint/no-this-alias -- the generator needs the feed
    const feed = this;
    return (async function* () {
      while (!signal.aborted) {
        const next = feed.queue.shift();
        if (next) {
          yield next;
        } else {
          await new Promise<void>((resolve) => (feed.wake = resolve));
        }
      }
    })();
  };

  push(event: LiveEventVm): void {
    this.queue.push(event);
    this.wake?.();
  }
}

describe('MapLive', () => {
  let feed: FakeFeed;
  let host: MapLiveHost &
    Record<'refresh' | 'visionChanged' | 'moveToken' | 'flagsChanged' | 'mapsChanged', Mock>;
  let live: MapLive;

  async function settle(ms = 0): Promise<void> {
    await vi.advanceTimersByTimeAsync(ms);
  }

  beforeEach(() => {
    vi.useFakeTimers();
    feed = new FakeFeed();
    host = {
      mapId: () => 'm1',
      isMaster: () => false,
      refresh: vi.fn(),
      visionChanged: vi.fn(),
      moveToken: vi.fn().mockReturnValue(true),
      flagsChanged: vi.fn(),
      mapsChanged: vi.fn(),
    };
    live = new MapLive({
      open: feed.open,
      classify: () => 'transient',
      document,
      host,
    });
  });

  afterEach(() => {
    live.stop();
    vi.useRealTimers();
  });

  async function start(): Promise<void> {
    live.start('c1');
    feed.push({ kind: 'ready' });
    await settle();
  }

  it('reads the map once for a burst of news about it', async () => {
    await start();
    feed.push({ kind: 'mapChanged', mapId: 'm1' });
    feed.push({ kind: 'visionChanged', mapId: 'm1' });
    feed.push({ kind: 'mapChanged', mapId: 'm1' });
    await settle(MAP_LIVE_DEBOUNCE_MS);
    expect(host.refresh).toHaveBeenCalledTimes(1);
    expect(host.visionChanged).toHaveBeenCalledTimes(1);
  });

  it('ignores news about another map', async () => {
    await start();
    feed.push({ kind: 'mapChanged', mapId: 'other' });
    feed.push({ kind: 'visionChanged', mapId: 'other' });
    feed.push({ kind: 'tokenMoved', mapId: 'other', characterId: 'x', xBp: 1, yBp: 2 });
    await settle(MAP_LIVE_DEBOUNCE_MS * 2);
    expect(host.refresh).not.toHaveBeenCalled();
    expect(host.visionChanged).not.toHaveBeenCalled();
    expect(host.moveToken).not.toHaveBeenCalled();
  });

  it('moves a token in place and only asks for the vision', async () => {
    await start();
    feed.push({ kind: 'tokenMoved', mapId: 'm1', characterId: 'k', xBp: 10, yBp: 20 });
    await settle(MAP_LIVE_DEBOUNCE_MS);
    expect(host.moveToken).toHaveBeenCalledWith('m1', 'k', 10, 20);
    expect(host.refresh).not.toHaveBeenCalled();
    expect(host.visionChanged).toHaveBeenCalledTimes(1);
  });

  it('reads the map when a moved token is not on it', async () => {
    host.moveToken.mockReturnValue(false);
    await start();
    feed.push({ kind: 'tokenMoved', mapId: 'm1', characterId: 'k', xBp: 10, yBp: 20 });
    await settle(MAP_LIVE_DEBOUNCE_MS);
    expect(host.refresh).toHaveBeenCalledTimes(1);
  });

  it('reads the map on a combat event, which names no map', async () => {
    await start();
    feed.push({ kind: 'combatantMoved', encounterId: 'e', combatantId: 'c', col: 1, row: 1 });
    await settle(MAP_LIVE_DEBOUNCE_MS);
    expect(host.refresh).toHaveBeenCalledTimes(1);
  });

  it('reads again after a reconnection, but not on the first ready', async () => {
    await start();
    await settle(MAP_LIVE_DEBOUNCE_MS);
    expect(host.refresh).not.toHaveBeenCalled();
    feed.push({ kind: 'ready' });
    await settle(MAP_LIVE_DEBOUNCE_MS);
    expect(host.refresh).toHaveBeenCalledTimes(1);
  });

  it('closes the stream on stop and does not read after it', async () => {
    await start();
    feed.push({ kind: 'mapChanged', mapId: 'm1' });
    await settle();
    live.stop();
    await settle(MAP_LIVE_DEBOUNCE_MS * 2);
    expect(feed.aborted).toBe(1);
    expect(host.refresh).not.toHaveBeenCalled();
  });

  it('keeps one stream for the same campaign and replaces it for another', async () => {
    await start();
    live.start('c1');
    expect(feed.opened).toBe(1);
    live.start('c2');
    await settle();
    expect(feed.opened).toBe(2);
    expect(feed.aborted).toBe(1);
  });

  it('opens again after the stream ended, when asked', async () => {
    await start();
    feed.push({ kind: 'ended' });
    await settle();
    expect(live.status).toBe('closed');
    live.ensure();
    await settle();
    expect(feed.opened).toBe(2);
  });
});
