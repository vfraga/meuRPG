import { Code, ConnectError } from '@connectrpc/connect';

import type { GetMapResponse } from '../../../gen/meurpg/maps/v1/maps_pb';
import { MapState } from './map-state';
import { mapMessage, mapPoint, mapResponse, mapToken } from './maps-testing';

function response(id: string, tokenX = 1000): GetMapResponse {
  return {
    map: { id, name: `Mapa ${id}`, pointCount: 1 },
    points: [{ id: `${id}-p`, name: 'Taverna' }],
    tokens: [{ characterId: 'c1', name: 'Pensantus', xBp: tokenX, yBp: 2000 }],
  } as unknown as GetMapResponse;
}

describe('MapState', () => {
  it('shows nothing without a map', async () => {
    const state = new MapState(() => Promise.reject(new Error('unused')));
    await state.open(null);
    expect(state.status()).toBe('idle');
    expect(state.map()).toBeNull();
  });

  it('reads the map, its points and its tokens', async () => {
    const state = new MapState(async (id) => response(id));
    await state.open('a');
    expect(state.status()).toBe('ready');
    expect(state.map()?.id).toBe('a');
    expect(state.points()).toHaveLength(1);
    expect(state.tokens()).toHaveLength(1);
  });

  it('means "the player lost sight of it" when the map is not found', async () => {
    const state = new MapState(async (id) => {
      if (id === 'hidden') {
        throw new ConnectError('nope', Code.NotFound);
      }
      return response(id);
    });
    await state.open('a');
    await state.open('hidden');
    expect(state.status()).toBe('gone');
    expect(state.map()).toBeNull();
    expect(state.points()).toEqual([]);
  });

  it('keeps the map on screen when a refetch fails for another reason', async () => {
    let fail = false;
    const state = new MapState(async (id) => {
      if (fail) {
        throw new ConnectError('down', Code.Unavailable);
      }
      return response(id);
    });
    await state.open('a');
    fail = true;
    await state.refresh();
    expect(state.status()).toBe('ready');
    expect(state.map()?.id).toBe('a');
  });

  it('reports an error on the first read that fails', async () => {
    const state = new MapState(async () => {
      throw new ConnectError('down', Code.Unavailable);
    });
    await state.open('a');
    expect(state.status()).toBe('error');
  });

  it('ignores an older answer that arrives after a newer one', async () => {
    const resolvers: Record<string, (r: GetMapResponse) => void> = {};
    const state = new MapState(
      (id) => new Promise<GetMapResponse>((resolve) => (resolvers[id] = resolve)),
    );
    const first = state.open('a');
    const second = state.open('b');
    resolvers['b'](response('b'));
    await second;
    resolvers['a'](response('a'));
    await first;
    expect(state.map()?.id).toBe('b');
  });

  it('moves a token without reading again, only on the open map', async () => {
    const state = new MapState(async (id) => response(id));
    await state.open('a');
    state.moveToken('a', 'c1', 4000, 5000);
    expect(state.tokens()[0]).toMatchObject({ xBp: 4000, yBp: 5000 });
    expect(state.moveToken('other', 'c1', 1, 1)).toBe(true);
    expect(state.tokens()[0]).toMatchObject({ xBp: 4000 });
  });

  it('says so when the moved token is not on the open map (read it again)', async () => {
    const state = new MapState(async (id) => response(id));
    await state.open('a');
    expect(state.moveToken('a', 'unknown', 1, 1)).toBe(false);
  });

  it('keeps the point count in step with the points', async () => {
    const state = new MapState(async (id) => response(id));
    await state.open('a');
    state.upsertPoint({ id: 'new', name: 'Nova' } as never);
    expect(state.map()?.pointCount).toBe(2);
    state.removePoint('new');
    expect(state.map()?.pointCount).toBe(1);
  });

  it("tells a creature's token from its owner's: they share a character_id, not a key (a light never replaces the creature)", () => {
    const state = new MapState(async () => mapResponse(mapMessage('map-1', 'M'), [], []));
    state.apply(
      mapResponse(
        mapMessage('map-1', 'M'),
        [],
        [
          mapToken('pensantus', 'Pensantus'),
          mapToken('pensantus', 'Nanquim', { creatureId: 'raven' }),
        ],
      ),
    );
    state.upsertToken(mapToken('pensantus', 'Pensantus', { carriedLight: 'light:torch' }));
    expect(state.tokens().map((t) => [t.name, t.carriedLight])).toEqual([
      ['Pensantus', 'light:torch'],
      ['Nanquim', ''],
    ]);
    // `token_moved` names the character: it moves the character's own token only.
    expect(state.moveToken('map-1', 'pensantus', 100, 200)).toBe(true);
    expect(state.tokens().map((t) => [t.name, t.xBp])).toEqual([
      ['Pensantus', 100],
      ['Nanquim', 4000],
    ]);
    state.removeToken('pensantus');
    expect(state.tokens().map((t) => t.name)).toEqual(['Nanquim']);
  });

  describe('a read in flight when the map is edited in place', () => {
    /** The first read answers at once; each later one waits for the test to answer it. */
    async function setup() {
      const pending: Array<(r: GetMapResponse) => void> = [];
      let first = true;
      const state = new MapState((id) => {
        if (first) {
          first = false;
          return Promise.resolve(response(id, 1000));
        }
        return new Promise<GetMapResponse>((resolve) => pending.push(resolve));
      });
      await state.open('a');
      const refreshing = state.refresh();
      return { state, pending, refreshing };
    }

    it('lands when nothing was edited meanwhile', async () => {
      const { state, pending, refreshing } = await setup();
      pending[0](response('a', 2000));
      await refreshing;
      expect(state.tokens()[0].xBp).toBe(2000);
      expect(pending).toHaveLength(1);
    });

    it('does not undo a token_moved: the older answer is dropped and the map is read again', async () => {
      const { state, pending, refreshing } = await setup();
      expect(state.moveToken('a', 'c1', 4000, 5000)).toBe(true);
      pending[0](response('a', 1000)); // served before the move committed
      await Promise.resolve();
      expect(state.tokens()[0]).toMatchObject({ xBp: 4000, yBp: 5000 });
      expect(pending).toHaveLength(2);
      pending[1](response('a', 4000));
      await refreshing;
      expect(state.tokens()[0].xBp).toBe(4000);
    });

    it("does not undo the master's own token or point answer", async () => {
      const { state, pending, refreshing } = await setup();
      state.upsertToken({ characterId: 'c1', name: 'Pensantus', xBp: 5000, yBp: 2000 } as never);
      state.upsertPoint({ id: 'a-p', name: 'Taverna', revealed: true } as never);
      pending[0](response('a', 1000));
      await Promise.resolve();
      expect(state.tokens()[0].xBp).toBe(5000);
      expect(state.points()[0].revealed).toBe(true);
      pending[1]({
        ...response('a', 5000),
        points: [{ id: 'a-p', name: 'Taverna', revealed: true }],
      } as never);
      await refreshing;
      expect(state.tokens()[0].xBp).toBe(5000);
    });

    it('does not read again when another map was opened meanwhile', async () => {
      const { state, pending, refreshing } = await setup();
      state.moveToken('a', 'c1', 4000, 5000);
      const other = state.open('b');
      pending[0](response('a', 1000));
      await refreshing;
      expect(pending).toHaveLength(2);
      pending[1](response('b'));
      await other;
      expect(state.map()?.id).toBe('b');
    });
  });

  describe('a row of another map', () => {
    it('is not put into the open map (a late answer after the map changed)', async () => {
      const state = new MapState(async (id) =>
        mapResponse(mapMessage(id, id), [], [mapToken('c1', 'Heroi', { mapId: id })]),
      );
      await state.open('map-b');
      state.upsertPoint(mapPoint('pa', 'Torre', { mapId: 'map-a' }));
      state.upsertToken(mapToken('c1', 'Heroi', { mapId: 'map-a', hidden: true }));
      expect(state.points()).toEqual([]);
      expect(state.map()?.pointCount).toBe(0);
      expect(state.tokens().map((t) => [t.mapId, t.hidden])).toEqual([['map-b', false]]);
    });

    it('is put into its own map', async () => {
      const state = new MapState(async (id) => mapResponse(mapMessage(id, id)));
      await state.open('map-a');
      state.upsertPoint(mapPoint('pa', 'Torre', { mapId: 'map-a' }));
      expect(state.points().map((p) => p.id)).toEqual(['pa']);
      expect(state.map()?.pointCount).toBe(1);
    });
  });
});
