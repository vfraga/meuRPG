import { Code, ConnectError } from '@connectrpc/connect';

import { MapReveals } from './map-reveals';
import { MapState } from './map-state';
import { FakeMapsClient, mapMessage, mapPoint, mapResponse, mapToken } from './maps-testing';

async function setup() {
  const api = new FakeMapsClient();
  const state = new MapState(async () =>
    mapResponse(
      mapMessage('map-1', 'Mirathel'),
      [mapPoint('p1', 'Torre')],
      [mapToken('t1', 'Goblin', { hidden: true })],
    ),
  );
  await state.open('map-1');
  const reveals = new MapReveals(
    api as never,
    () => state,
    () => 'camp-1',
  );
  return { api, state, reveals };
}

describe('MapReveals', () => {
  it('reveals a point and puts the answer into the map', async () => {
    const { api, state, reveals } = await setup();
    await reveals.togglePoint(state.points()[0], true);
    expect(api.calls).toContain('setPointRevealed map-1 p1 true');
    expect(state.points()[0].revealed).toBe(true);
    expect(reveals.announcement()).toBe('Torre foi revelado aos jogadores.');
    expect(reveals.pendingId()).toBeNull();
  });

  it('shows a hidden token to the players', async () => {
    const { api, state, reveals } = await setup();
    await reveals.toggleToken(state.tokens()[0], false);
    expect(api.calls).toContain('setTokenHidden map-1 t1 false');
    expect(state.tokens()[0].hidden).toBe(false);
  });

  it('keeps the state and says what happened when the call fails', async () => {
    const { api, state, reveals } = await setup();
    api.failWith = new ConnectError('no', Code.PermissionDenied);
    await reveals.togglePoint(state.points()[0], true);
    expect(state.points()[0].revealed).toBe(false);
    expect(reveals.error()).toContain('Só o mestre');
    expect(reveals.pendingId()).toBeNull();
  });

  it("never hides a creature's token: its character is the owner's, whose token would change", async () => {
    const { api, state, reveals } = await setup();
    const creature = mapToken('t1', 'Corvo', { creatureId: 'raven' });
    await reveals.toggleToken(creature, true);
    expect(api.calls.filter((c) => c.startsWith('setTokenHidden'))).toEqual([]);
    expect(state.tokens()[0].hidden).toBe(true);
  });

  describe('when the map changes before the answer', () => {
    async function switching() {
      let resolvePoint!: (p: ReturnType<typeof mapPoint>) => void;
      let resolveToken!: (t: ReturnType<typeof mapToken>) => void;
      const api = {
        setPointRevealed: () => new Promise<ReturnType<typeof mapPoint>>((r) => (resolvePoint = r)),
        setTokenHidden: () => new Promise<ReturnType<typeof mapToken>>((r) => (resolveToken = r)),
      };
      const state = new MapState(async (id) =>
        id === 'map-A'
          ? mapResponse(
              mapMessage('map-A', 'A'),
              [mapPoint('pA', 'Ponto A', { mapId: 'map-A' })],
              [mapToken('char-1', 'Heroi A', { mapId: 'map-A' })],
            )
          : mapResponse(
              mapMessage('map-B', 'B', { pointCount: 1 }),
              [mapPoint('pB', 'Ponto B', { mapId: 'map-B' })],
              [mapToken('char-1', 'Heroi B', { mapId: 'map-B' })],
            ),
      );
      await state.open('map-A');
      const reveals = new MapReveals(
        api as never,
        () => state,
        () => 'camp-1',
      );
      return {
        state,
        reveals,
        answerPoint: (p: ReturnType<typeof mapPoint>) => resolvePoint(p),
        answerToken: (t: ReturnType<typeof mapToken>) => resolveToken(t),
      };
    }

    it('puts the answer into the map it was asked for when the map did not change', async () => {
      const { state, reveals, answerPoint } = await switching();
      const done = reveals.togglePoint(state.points()[0], true);
      answerPoint(mapPoint('pA', 'Ponto A', { mapId: 'map-A', revealed: true }));
      await done;
      expect(state.points().map((p) => [p.id, p.revealed])).toEqual([['pA', true]]);
    });

    it('does not put the point of the old map into the new one', async () => {
      const { state, reveals, answerPoint } = await switching();
      const done = reveals.togglePoint(state.points()[0], true);
      await state.open('map-B');
      answerPoint(mapPoint('pA', 'Ponto A', { mapId: 'map-A', revealed: true }));
      await done;
      expect(state.points().map((p) => p.id)).toEqual(['pB']);
      expect(state.map()?.pointCount).toBe(1);
      expect(reveals.error()).toBeNull();
    });

    it('does not replace the new map token with the old map one', async () => {
      const { state, reveals, answerToken } = await switching();
      const done = reveals.toggleToken(state.tokens()[0], true);
      await state.open('map-B');
      answerToken(mapToken('char-1', 'Heroi A', { mapId: 'map-A', hidden: true }));
      await done;
      expect(state.tokens().map((t) => [t.mapId, t.hidden])).toEqual([['map-B', false]]);
    });
  });
});
