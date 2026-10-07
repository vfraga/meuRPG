// Finding U13-25 (review/unit-13-web-live-rest.md): MapReveals applies an answer from map A into the state after the master opened map B.
import type { MapPoint, MapToken } from '../../../gen/meurpg/maps/v1/maps_pb';
import { MapReveals } from './map-reveals';
import { MapState } from './map-state';
import { mapMessage, mapPoint, mapResponse, mapToken } from './maps-testing';

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

async function setup() {
  const pointCall = deferred<MapPoint>();
  const tokenCall = deferred<MapToken>();
  const api = {
    setPointRevealed: () => pointCall.promise,
    setTokenHidden: () => tokenCall.promise,
  };
  const state = new MapState(async (id) =>
    id === 'map-A'
      ? mapResponse(mapMessage('map-A', 'A'), [mapPoint('pA', 'Ponto A', { mapId: 'map-A' })])
      : mapResponse(
          mapMessage('map-B', 'B'),
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
  return { state, reveals, pointCall, tokenCall };
}

describe('Review13 U13-25: late answer from the old map lands in the new map', () => {
  it('control: the answer lands in the same map when the map did not change', async () => {
    const { state, reveals, pointCall } = await setup();
    const p = reveals.togglePoint(state.points()[0], true);
    pointCall.resolve(mapPoint('pA', 'Ponto A', { mapId: 'map-A', revealed: true }));
    await p;
    expect(state.points().map((x) => x.id)).toEqual(['pA']);
    expect(state.points()[0].revealed).toBe(true);
  });

  it('does not put a map A point into map B', async () => {
    const { state, reveals, pointCall } = await setup();
    const p = reveals.togglePoint(state.points()[0], true);
    await state.open('map-B');
    expect(state.points().map((x) => x.id)).toEqual(['pB']);
    pointCall.resolve(mapPoint('pA', 'Ponto A', { mapId: 'map-A', revealed: true }));
    await p;
    expect(state.points().map((x) => x.id)).toEqual(['pB']);
    expect(state.map()?.pointCount).not.toBe(2);
  });

  it('does not replace map B token with the map A token', async () => {
    const { state, reveals, tokenCall } = await setup();
    const t = reveals.toggleToken(mapToken('char-1', 'Heroi A', { mapId: 'map-A' }), true);
    await state.open('map-B');
    tokenCall.resolve(mapToken('char-1', 'Heroi A', { mapId: 'map-A', hidden: true }));
    await t;
    expect(state.tokens().map((x) => x.mapId)).toEqual(['map-B']);
  });
});
